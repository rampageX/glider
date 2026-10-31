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
	conf       Config
	paths      []route
	mu         sync.Mutex
	statusMu   sync.RWMutex
	status     Status
	needsRetry bool
	apply      func([]string, func() error) (bool, error)
}

func New(c Config, apply func([]string, func() error) (bool, error)) (*Manager, error) {
	if len(c.Titles) != 0 && len(c.Titles) != len(c.URLs) {
		return nil, errors.New("subscription-title count must match subscription count")
	}
	if c.Interval < 1 || c.RetryInterval < 0 {
		return nil, errors.New("invalid refresh/retry interval")
	}
	paths, e := routes(c.Paths)
	if e != nil {
		return nil, e
	}
	for _, raw := range c.URLs {
		if !validURL(raw) {
			return nil, errors.New("invalid subscription URL")
		}
	}
	m := &Manager{conf: c, paths: paths, apply: apply}
	m.initStatus()
	return m, nil
}
func validURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}

// Run serializes startup, timer and signal-triggered refreshes. Triggers coalesce.
func (m *Manager) Run(ctx context.Context, triggers <-chan os.Signal) {
	defer m.setNext(nil)
	for {
		if ctx.Err() != nil {
			return
		}
		m.Refresh(ctx)
		if ctx.Err() != nil {
			return
		}
		delay := m.nextDelay()
		next := time.Now().Add(delay)
		m.setNext(&next)
		log.F("[subscription] next refresh in %d seconds", int(delay/time.Second))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case <-triggers:
			timer.Stop()
		}
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
	m.beginRefresh()
	defer m.endRefresh()
	var all []Node
	successes := 0
	for i, raw := range m.conf.URLs {
		if ctx.Err() != nil {
			return
		}
		m.beginInput(i)
		result, e := m.fetchInput(ctx, raw)
		m.endInput(i, result, e)
		nodes, hit, skipped := result.nodes, result.hit, result.skipped
		if skipped > 0 {
			log.F("[subscription] input %d skipped %d unsupported nodes (scheme/plugin/cipher/protocol/obfs)", i+1, skipped)
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
		result := "RETAINED_EMPTY"
		if successes == 0 {
			result = "RETAINED_NO_INPUT"
		}
		m.setApply(result, false)
		log.F("[subscription] no usable candidate; retaining current nodes")
		return
	}
	if ctx.Err() != nil {
		return
	}
	if e := Validate(defs); e != nil {
		m.setApply("VALIDATION_FAILED", true)
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
		m.setApply("PERSIST_FAILED", true)
		log.F("[subscription] commit failed; retaining current nodes")
		return
	}
	if !changed {
		m.setApply("UNCHANGED", false)
		log.F("[subscription] content unchanged; nodes and file retained")
		return
	}
	m.setApply("UPDATED", false)
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

type inputResult struct {
	nodes   []Node
	hit     bool
	skipped int
	at      time.Time
}

// Retain the small input interface used by existing cache tests.
func (m *Manager) input(ctx context.Context, raw string) ([]Node, bool, int, error) {
	r, e := m.fetchInput(ctx, raw)
	return r.nodes, r.hit, r.skipped, e
}
func (m *Manager) fetchInput(ctx context.Context, raw string) (inputResult, error) {
	path := filepath.Join(m.conf.CacheDir, cacheKey(raw)+".json")
	read := func() (inputResult, error) {
		b, e := os.ReadFile(path)
		if e != nil {
			return inputResult{}, e
		}
		if len(b) > m.conf.MaxBytes*2+4096 {
			return inputResult{}, errors.New("cache too large")
		}
		var entry cacheEntry
		if json.Unmarshal(b, &entry) != nil {
			return inputResult{}, errors.New("invalid cache")
		}
		age := time.Since(entry.At)
		if age < 0 || age >= m.conf.duration(m.conf.TTL) {
			return inputResult{}, errors.New("expired cache")
		}
		n, s, e := Parse(entry.Body)
		return inputResult{nodes: n, hit: true, skipped: s, at: entry.At}, e
	}
	if r, e := read(); e == nil {
		return r, nil
	}
	if e := os.MkdirAll(m.conf.CacheDir, 0700); e != nil {
		return inputResult{}, e
	}
	unlock, e := lock(ctx, path+".lock")
	if e != nil {
		return inputResult{}, e
	}
	defer unlock()
	if r, e := read(); e == nil {
		return r, nil
	}
	skipped := 0
	for index, r := range m.paths {
		if ctx.Err() != nil {
			return inputResult{}, ctx.Err()
		}
		debugf(m.conf, "input=%s route=%d via=%s stage=get-start", cacheKey(raw)[:8], index+1, routeLabel(r.definition))
		b, e := download(ctx, raw, r, m.conf)
		if e != nil {
			log.F("[subscription] input %s route %d GET failed", cacheKey(raw)[:8], index+1)
			debugf(m.conf, "input=%s route=%d %s", cacheKey(raw)[:8], index+1, e.Error())
			continue
		}
		n, s, e := Parse(b)
		if s > skipped {
			skipped = s
		}
		if e != nil {
			log.F("[subscription] input %s route %d invalid/unsupported input; ignored in full", cacheKey(raw)[:8], index+1)
			debugf(m.conf, "input=%s route=%d stage=parse-result reason=%s", cacheKey(raw)[:8], index+1, e.Error())
			continue
		}
		defs := make([]string, len(n))
		for i := range n {
			defs[i] = n[i].Forward
		}
		if Validate(defs) != nil {
			debugf(m.conf, "input=%s route=%d stage=forwarder-validation reason=invalid-forwarder", cacheKey(raw)[:8], index+1)
			continue
		}
		entry := cacheEntry{At: time.Now(), Body: b}
		data, e := json.Marshal(entry)
		if e != nil {
			return inputResult{}, e
		}
		if e = atomicWrite(path, data); e != nil {
			return inputResult{}, e
		}
		return inputResult{nodes: n, skipped: s, at: entry.At}, nil
	}
	return inputResult{skipped: skipped}, errors.New("all GET routes failed")
}
