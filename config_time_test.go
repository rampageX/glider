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

func TestLHAHistoryRetentionDaysConfigAndOverrides(t *testing.T) {
	oldArgs, oldFlag := os.Args, flag
	defer func() { os.Args = oldArgs; flag = oldFlag }()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "common.conf"), []byte("lha-history-retention-days=1w\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, body string
		cli        []string
		want       int
	}{
		{"default", "listen=127.0.0.1:7575\nforward=direct://\n", nil, 3},
		{"config-file", "listen=127.0.0.1:7575\nforward=direct://\nlha-history-retention-days=2d\n", nil, 2},
		{"included-config", "include=common.conf\nlisten=127.0.0.1:7575\nforward=direct://\n", nil, 7},
		{"command-line-override", "include=common.conf\nlisten=127.0.0.1:7575\nforward=direct://\n", []string{"-lha-history-retention-days=0"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".conf")
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil { t.Fatal(err) }
			flag = conflag.New("test")
			os.Args = append([]string{"glider", "-config="+path}, tc.cli...)
			conf := parseConfig()
			if conf.HistoryRetentionDays != tc.want {
				t.Fatalf("lha-history-retention-days: got %d want %d", conf.HistoryRetentionDays, tc.want)
			}
		})
	}
}

func TestLHAHistoryEnabledConfigAndOverrides(t *testing.T) {
	oldArgs, oldFlag := os.Args, flag
	defer func() { os.Args = oldArgs; flag = oldFlag }()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "common.conf"), []byte("lha-history-enabled=false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, body string
		cli        []string
		want       bool
	}{
		{"default", "listen=127.0.0.1:7575\nforward=direct://\n", nil, true},
		{"config-false", "listen=127.0.0.1:7575\nforward=direct://\nlha-history-enabled=false\n", nil, false},
		{"common-include", "include=common.conf\nlisten=127.0.0.1:7575\nforward=direct://\n", nil, false},
		{"cli-enables", "include=common.conf\nlisten=127.0.0.1:7575\nforward=direct://\n", []string{"-lha-history-enabled=true"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".conf")
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			flag = conflag.New("test")
			os.Args = append([]string{"glider", "-config="+path}, tc.cli...)
			c := parseConfig()
			if c.HistoryEnabled != tc.want {
				t.Fatalf("lha-history-enabled: got %t want %t", c.HistoryEnabled, tc.want)
			}
		})
	}
}
