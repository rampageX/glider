package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/nadoo/glider/dns"
	"github.com/nadoo/glider/ipset"
	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/rule"
	"github.com/nadoo/glider/service"
	"github.com/nadoo/glider/stats"
	"github.com/nadoo/glider/subscription"
	"github.com/nadoo/glider/web"
)

var (
	version = "0.22.0"
	config  *Config
)

func main() {
	config = parseConfig()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	static := append([]string(nil), config.Forwards...)
	if len(config.Subscription.URLs) > 0 {
		if defs, err := subscription.ReadSnapshot(config.Subscription.Output); err == nil && subscription.Validate(defs) == nil {
			config.Forwards = append(config.Forwards, defs...)
			log.F("[subscription] restored last successful node snapshot; freshness unknown")
		} else if len(config.Forwards) == 0 {
			config.Forwards = []string{"reject://#name=subscription-pending"}
			log.F("[subscription] no valid snapshot; waiting for subscription (fail closed)")
		}
	}
	if err := stats.InitAvailabilityStore(config.HistoryDir, lhaHistoryInstanceID(config)); err != nil {
		log.F("[lha-history] historical availability will not be persisted: %s", err)
	} else {
		defer stats.CloseAvailabilityStore()
	}

	pxy := rule.NewProxy(config.Forwards, &config.Strategy, config.rules)
	defer pxy.Close()
	var manager *subscription.Manager
	if len(config.Subscription.URLs) > 0 {
		pxy.EnableDynamicMain()
		var err error
		manager, err = subscription.New(config.Subscription, func(defs []string, persist func() error) (bool, error) {
			return pxy.ReplaceMain(append(append([]string(nil), static...), defs...), persist)
		})
		if err != nil {
			log.Fatal(err)
		}
	}
	ipsetM, _ := ipset.NewManager(config.rules)

	if config.DNS != "" {
		d, err := dns.NewServer(config.DNS, pxy, &config.DNSConfig)
		if err != nil {
			log.Fatal(err)
		}
		for _, r := range config.rules {
			if len(r.DNSServers) > 0 {
				for _, domain := range r.Domain {
					d.SetServers(domain, r.DNSServers)
				}
			}
		}
		d.AddHandler(pxy.AddDomainIP)
		if ipsetM != nil {
			d.AddHandler(ipsetM.AddDomainIP)
		}
		d.Start()
		net.DefaultResolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: time.Second * 3}
				return d.DialContext(ctx, "udp", config.DNS)
			},
		}
	}

	for _, r := range config.rules {
		r.IP, r.CIDR, r.Domain = nil, nil, nil
	}

	pxy.Check()
	if config.Web != "" {
		var provider func() subscription.Status
		if manager != nil {
			provider = manager.Snapshot
		}
		go web.New(config.Web, pxy, provider).ListenAndServe()
	}

	for _, listen := range config.Listens {
		local, err := proxy.ServerFromURL(listen, pxy)
		if err != nil {
			log.Fatal(err)
		}
		go local.ListenAndServe()
	}

	for _, s := range config.Services {
		service, err := service.New(s)
		if err != nil {
			log.Fatal(err)
		}
		go service.Run()
	}

	if manager != nil {
		refresh := make(chan os.Signal, 1)
		signal.Notify(refresh, syscall.SIGHUP)
		defer signal.Stop(refresh)
		done := make(chan struct{})
		go func() { defer close(done); manager.Run(ctx, refresh) }()
		<-ctx.Done()
		<-done
	} else {
		<-ctx.Done()
	}

}

// lhaHistoryInstanceID keeps histories from differently configured Glider
// instances separate while remaining stable across restarts. Hashing avoids
// putting service addresses or other configuration values in the directory name.
func lhaHistoryInstanceID(conf *Config) string {
	identity := struct {
		Listens  []string
		Web      string
		DNS      string
		Services []string
	}{
		Listens:  append([]string(nil), conf.Listens...),
		Web:      conf.Web,
		DNS:      conf.DNS,
		Services: append([]string(nil), conf.Services...),
	}
	sort.Strings(identity.Listens)
	sort.Strings(identity.Services)
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:6])
}
