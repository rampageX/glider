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
	"github.com/nadoo/glider/web"
)

var (
	version = "0.21.0"
	config  = parseConfig()
)

func main() {
	if err := stats.InitAvailabilityStore(flag.ConfDir(), lhaHistoryInstanceID(config)); err != nil {
		log.F("[lha-history] historical availability will not be persisted: %s", err)
	} else {
		defer stats.CloseAvailabilityStore()
	}

	pxy := rule.NewProxy(config.Forwards, &config.Strategy, config.rules)
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
		go web.New(config.Web, pxy).ListenAndServe()
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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
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
