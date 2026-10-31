package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nadoo/conflag"
	"github.com/nadoo/glider/subscription"
)

func TestShippedSubscriptionConfigurations(t *testing.T) {
	oldArgs, oldFlag := os.Args, flag
	defer func() { os.Args = oldArgs; flag = oldFlag }()
	for _, region := range []string{"asia", "hk", "jp", "us"} {
		t.Run(region, func(t *testing.T) {
			dir := t.TempDir()
			common, e := os.ReadFile("config/subscription/common.conf.example")
			if e != nil {
				t.Fatal(e)
			}
			os.WriteFile(filepath.Join(dir, "subscription-common.conf"), common, 0600)
			body, e := os.ReadFile("config/subscription/" + region + ".conf.example")
			if e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(dir, "main.conf")
			os.WriteFile(path, body, 0600)
			flag = conflag.New("test")
			os.Args = []string{"glider", "-config=" + path}
			c := parseConfig()
			if c.Subscription.Region != region || len(c.Subscription.URLs) != 2 || len(c.Subscription.Paths) != 6 || len(c.Forwards) != 0 {
				t.Fatalf("unexpected config %+v", c.Subscription)
			}
			if _, e := subscription.New(c.Subscription, nil); e != nil {
				t.Fatal(e)
			}
			if !strings.HasSuffix(c.Subscription.Output, "glider_db_"+region+"_ssr.conf") {
				t.Fatal(c.Subscription.Output)
			}
			if c.HistoryDir != "/tmp/.glider-history" {
				t.Fatal(c.HistoryDir)
			}
			if c.Subscription.CacheDir != "/tmp/.subscription-cache" {
				t.Fatal(c.Subscription.CacheDir)
			}
			if len(c.Subscription.Titles) != 2 || c.Subscription.Titles[0] != "订阅一" || c.Subscription.RetryInterval != 7200 {
				t.Fatal("titles/retry not loaded")
			}
			if c.Subscription.TTL != 3600 || c.Subscription.Interval != 3600 {
				t.Fatal("limits not loaded")
			}
		})
	}
}
