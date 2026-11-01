package subscription

import (
	"errors"
	"github.com/nadoo/conflag"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Titles                                           []string
	Protocols                                        []string
	URLs, Matches, Excludes, Paths                   []string
	Region, Output, CacheDir                         string
	Interval, TTL, ConnectTimeout, Timeout, MaxBytes int
	RetryInterval                                    int
}

func RegisterFlags(f *conflag.Conflag, c *Config) {
	f.StringSliceVar(&c.Titles, "subscription-title", nil, "optional subscription titles, one per subscription in the same order")
	f.IntVar(&c.RetryInterval, "subscription-retry-interval", 7200, "retry failed refresh after seconds; 0 disables extra retries")
	f.StringSliceVar(&c.URLs, "subscription", nil, "subscription URL (repeatable)")
	f.StringSliceVar(&c.Protocols, "subscription-protocol", nil, "allowed subscription protocols: ss, ssr, anytls, trojan, vless (repeatable or comma-separated; empty allows all)")
	f.StringSliceVar(&c.Matches, "subscription-match", nil, "literal case-sensitive include keyword (OR)")
	f.StringSliceVar(&c.Excludes, "subscription-exclude", []string{"遊戲"}, "literal exclude keyword (repeatable; empty disables default)")
	f.StringSliceVar(&c.Paths, "subscription-proxy", []string{"direct://"}, "download forwarder URL, larger #priority first; stable ties")
	f.StringVar(&c.Region, "subscription-region", "", "instance region label")
	f.StringVar(&c.Output, "subscription-output", "", "managed forward file; DO NOT include it")
	f.StringVar(&c.CacheDir, "subscription-cache-dir", filepath.Join(os.TempDir(), ".subscription-cache"), "shared raw subscription cache directory")
	f.IntVar(&c.Interval, "subscription-interval", 3600, "refresh interval seconds, independent of cache TTL")
	f.IntVar(&c.TTL, "subscription-cache-ttl", 3600, "raw cache validity seconds")
	f.IntVar(&c.ConnectTimeout, "subscription-connect-timeout", 3, "download connection/handshake timeout seconds")
	f.IntVar(&c.Timeout, "subscription-timeout", 15, "total timeout per GET route seconds")
	f.IntVar(&c.MaxBytes, "subscription-max-bytes", 8<<20, "maximum response bytes")
}
func (c *Config) Resolve(dir string) error {
	if err := c.normalizeProtocols(); err != nil {
		return err
	}
	if len(c.URLs) == 0 {
		if c.Output != "" {
			return errors.New("subscription-output requires subscription")
		}
		return nil
	}
	if c.Output == "" {
		return errors.New("subscription-output is required")
	}
	if len(c.Titles) != 0 && len(c.Titles) != len(c.URLs) {
		return errors.New("subscription-title count must match subscription count")
	}
	if c.RetryInterval < 0 {
		return errors.New("subscription-retry-interval cannot be negative")
	}
	if c.Interval < 1 || c.TTL < 0 || c.ConnectTimeout < 1 || c.Timeout < 1 || c.MaxBytes < 1 {
		return errors.New("invalid subscription limits")
	}
	for _, p := range []*string{&c.Output, &c.CacheDir} {
		if !filepath.IsAbs(*p) {
			*p = filepath.Join(dir, *p)
		}
	}
	return nil
}
func (c Config) duration(v int) time.Duration { return time.Duration(v) * time.Second }
