package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nadoo/conflag"
	"github.com/nadoo/glider/rule"
)

func TestDurationUnitsInCommonAndRuleConfigs(t *testing.T) {
	oldArgs, oldFlag := os.Args, flag
	defer func() { os.Args = oldArgs; flag = oldFlag }()
	dir := t.TempDir()
	common := "checkinterval=2m\nchecktimeout=5\ndialtimeout=1m\nrelaytimeout=1h\nchecktolerance=1s\ndnstimeout=1m\ndnsmaxttl=1d\ndnsminttl=2m\nsubscription-interval=2h\nsubscription-cache-ttl=1w\nsubscription-retry-interval=1d\nsubscription-connect-timeout=1m\nsubscription-timeout=2m\n"
	if err := os.WriteFile(filepath.Join(dir, "common.conf"), []byte(common), 0600); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.conf")
	if err := os.WriteFile(main, []byte("include=common.conf\nlisten=127.0.0.1:7575\nforward=direct://\n"), 0600); err != nil {
		t.Fatal(err)
	}
	flag = conflag.New("test")
	os.Args = []string{"glider", "-config=" + main}
	c := parseConfig()
	if c.Strategy.CheckInterval != 120 || c.Strategy.CheckTimeout != 5 || c.Strategy.DialTimeout != 60 || c.Strategy.RelayTimeout != 3600 || c.Strategy.CheckTolerance != 1000 {
		t.Fatal("strategy time settings incorrect")
	}
	if c.DNSConfig.Timeout != 60 || c.DNSConfig.MaxTTL != 86400 || c.DNSConfig.MinTTL != 120 {
		t.Fatal("DNS time settings incorrect")
	}
	sub := c.Subscription
	if sub.Interval != 7200 || sub.TTL != 604800 || sub.RetryInterval != 86400 || sub.ConnectTimeout != 60 || sub.Timeout != 120 {
		t.Fatal("subscription time settings incorrect")
	}
	rulePath := filepath.Join(dir, "test.rule")
	os.WriteFile(rulePath, []byte("checkinterval=1h\nchecktimeout=2m\ndialtimeout=1m\nrelaytimeout=1d\nchecktolerance=500\n"), 0600)
	r, err := rule.NewConfFromFile(rulePath)
	if err != nil || r.Strategy.CheckInterval != 3600 || r.Strategy.CheckTimeout != 120 || r.Strategy.DialTimeout != 60 || r.Strategy.RelayTimeout != 86400 || r.Strategy.CheckTolerance != 500 {
		t.Fatal("rule config durations incorrect", err)
	}
}
