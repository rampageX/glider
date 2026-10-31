package subscription

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/rule"
)

type Manager struct {
	conf  Config
	paths []route
	mu    sync.Mutex
	apply func([]string, func() error) (bool, error)
}

func New(c Config, apply func([]string, func() error) (bool, error)) (*Manager, error) {
	paths, e := routes(c.Paths)
	if e != nil {
		return nil, e
	}
	for _, raw := range c.URLs {
		if !validURL(raw) {
			return nil, errors.New("invalid subscription URL")
		}
	}
	return &Manager{conf: c, paths: paths, apply: apply}, nil
}
func validURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}

// Run serializes startup, timer and signal-triggered refreshes. Triggers coalesce.
func (m *Manager) Run(ctx context.Context, triggers <-chan os.Signal) {
	m.Refresh(ctx)
	timer := time.NewTicker(m.conf.duration(m.conf.Interval))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-triggers:
		}
		if ctx.Err() != nil {
			return
		}
		m.Refresh(ctx)
	}
}
func Validate(defs []string) error {
	for _, s := range defs {
		if _, e := rule.ForwarderFromURL(s, "", time.Second, time.Second); e != nil {
			return errors.New("unsupported or invalid forwarder")
		}
	}
	return nil
}
func (m *Manager) Refresh(ctx context.Context) {
	// Also guard direct/manual callers, without queueing redundant refreshes.
	if !m.mu.TryLock() {
		return
	}
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	var all []Node
	successes := 0
	for i, raw := range m.conf.URLs {
		if ctx.Err() != nil {
			return
		}
		nodes, hit, skipped, e := m.input(ctx, raw)
		if skipped > 0 {
			log.F("[subscription] input %d skipped %d unsupported nodes (plugin/cipher/protocol/obfs)", i+1, skipped)
		}
		if e != nil {
			log.F("[subscription] input %d failed; ignored", i+1)
			continue
		}
		successes++
		all = append(all, nodes...)
		if hit {
			log.F("[subscription] input %d cache hit", i+1)
		} else {
			log.F("[subscription] input %d refreshed", i+1)
		}
	}
	defs := Filter(all, m.conf.Matches, m.conf.Excludes)
	if successes == 0 || len(defs) == 0 {
		log.F("[subscription] no usable candidate; retaining current nodes")
		return
	}
	if ctx.Err() != nil {
		return
	}
	if e := Validate(defs); e != nil {
		log.F("[subscription] candidate invalid; retaining current nodes")
		return
	}
	data := Render(defs)
	changed, e := m.apply(defs, func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return atomicWrite(m.conf.Output, data)
	})
	if e != nil {
		log.F("[subscription] commit failed; retaining current nodes")
		return
	}
	if !changed {
		log.F("[subscription] content unchanged; nodes and file retained")
		return
	}
	if successes < len(m.conf.URLs) {
		log.F("[subscription] partial success: %d/%d inputs, applied %d nodes", successes, len(m.conf.URLs), len(defs))
	} else {
		log.F("[subscription] applied %d nodes", len(defs))
	}
}

type cacheEntry struct {
	At   time.Time
	Body []byte
}

func cacheKey(raw string) string { h := sha256.Sum256([]byte(raw)); return hex.EncodeToString(h[:]) }
func (m *Manager) input(ctx context.Context, raw string) ([]Node, bool, int, error) {
	path := filepath.Join(m.conf.CacheDir, cacheKey(raw)+".json")
	read := func() ([]Node, int, error) {
		b, e := os.ReadFile(path)
		if e != nil {
			return nil, 0, e
		}
		if len(b) > m.conf.MaxBytes*2+4096 {
			return nil, 0, errors.New("cache too large")
		}
		var entry cacheEntry
		if json.Unmarshal(b, &entry) != nil {
			return nil, 0, errors.New("invalid cache")
		}
		age := time.Since(entry.At)
		if age < 0 || age >= m.conf.duration(m.conf.TTL) {
			return nil, 0, errors.New("expired cache")
		}
		return Parse(entry.Body)
	}
	if n, s, e := read(); e == nil {
		return n, true, s, nil
	}
	if e := os.MkdirAll(m.conf.CacheDir, 0700); e != nil {
		return nil, false, 0, e
	}
	unlock, e := lock(ctx, path+".lock")
	if e != nil {
		return nil, false, 0, e
	}
	defer unlock()
	if n, s, e := read(); e == nil {
		return n, true, s, nil
	}
	skipped := 0
	for index, r := range m.paths {
		if ctx.Err() != nil {
			return nil, false, 0, ctx.Err()
		}
		debugf(m.conf,"input=%s route=%d via=%s stage=get-start",cacheKey(raw)[:8],index+1,routeLabel(r.definition))
 b, e := download(ctx, raw, r, m.conf)
		if e != nil {
			log.F("[subscription] input %s route %d GET failed", cacheKey(raw)[:8], index+1)
 debugf(m.conf,"input=%s route=%d %s",cacheKey(raw)[:8],index+1,e.Error())
			continue
		}
		n, s, e := Parse(b)
		if s > skipped {
			skipped = s
		}
		if e != nil {
			log.F("[subscription] input %s route %d invalid/unsupported input; ignored in full", cacheKey(raw)[:8], index+1)
 debugf(m.conf,"input=%s route=%d stage=parse-result reason=%s",cacheKey(raw)[:8],index+1,e.Error())
			continue
		}
		defs := make([]string, len(n))
		for i := range n {
			defs[i] = n[i].Forward
		}
		if Validate(defs) != nil {
 debugf(m.conf,"input=%s route=%d stage=forwarder-validation reason=invalid-forwarder",cacheKey(raw)[:8],index+1)
			continue
		}
		entry := cacheEntry{At: time.Now(), Body: b}
		data, e := json.Marshal(entry)
		if e != nil {
			return nil, false, 0, e
		}
		if e = atomicWrite(path, data); e != nil {
			return nil, false, 0, e
		}
		return n, false, s, nil
	}
	return nil, false, skipped, errors.New("all GET routes failed")
}
