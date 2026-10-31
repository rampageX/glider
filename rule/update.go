package rule

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

func (p *FwdrGroup) startCheckLocked(f *Forwarder) {
	if p.checker == nil || p.closed {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.checks[f] = cancel
	p.checkWG.Add(1)
	go p.check(ctx, f, p.checker)
}

// ReplaceMain validates every definition before persist. persist must atomically
// publish the managed file. After it succeeds the in-memory commit cannot fail.
// Connections already handed to callers are never closed here.
func (p *Proxy) ReplaceMain(definitions []string, persist func() error) (bool, error) {
	changed, err := p.main.replace(definitions, persist)
	if changed {
		p.updateForwardRoutes()
	}
	return changed, err
}
func (p *FwdrGroup) replace(defs []string, persist func() error) (bool, error) {
	if len(defs) == 0 {
		return false, errors.New("empty candidate")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false, errors.New("group closed")
	}
	if len(defs) == len(p.fwdrs) {
		same := true
		for i, s := range defs {
			if p.fwdrs[i].URL() != s {
				same = false
				break
			}
		}
		if same {
			return false, nil
		}
	}
	// Queue by exact definition, preserving duplicate occurrences and state.
	old := map[string][]*Forwarder{}
	for _, f := range p.fwdrs {
		old[f.URL()] = append(old[f.URL()], f)
	}
	next := make(priSlice, 0, len(defs))
	added := map[*Forwarder]bool{}
	for _, s := range defs {
		var f *Forwarder
		if list := old[s]; len(list) > 0 {
			f = list[0]
			old[s] = list[1:]
		} else {
			var err error
			f, err = ForwarderFromURL(s, p.config.IntFace, time.Duration(p.config.DialTimeout)*time.Second, time.Duration(p.config.RelayTimeout)*time.Second)
			if err != nil {
				return false, errors.New("invalid candidate forwarder")
			}
			f.SetMaxFailures(uint32(p.config.MaxFailures))
			added[f] = true
		}
		next = append(next, f)
	}
	if persist != nil {
		if err := persist(); err != nil {
			return false, err
		}
	}
	retained := map[*Forwarder]bool{}
	for _, f := range next {
		retained[f] = true
	}
	for _, f := range p.fwdrs {
		if !retained[f] {
			if cancel := p.checks[f]; cancel != nil {
				cancel()
				delete(p.checks, f)
			}
		}
	}
	p.fwdrs = next
	p.init()
	p.selectionMu.Lock()
	if !retained[p.current] {
		p.current = nil
	}
	p.selectionMu.Unlock()
	for _, f := range next {
		if added[f] {
			f.AddHandler(p.onStatusChanged)
			p.startCheckLocked(f)
		}
	}
	return true, nil
}
func (p *Proxy) Close() {
	p.main.close()
	for _, g := range p.all {
		g.close()
	}
}
func (p *FwdrGroup) close() {
	p.mu.Lock()
	p.closed = true
	for _, cancel := range p.checks {
		cancel()
	}
	p.mu.Unlock()
	p.checkWG.Wait()
}

// Each published route table owns its learned IPs, so removed node host rules
// and DNS-derived direct routes retire together on the next update.
type forwardRoutes struct {
	hosts map[string]bool
	ips   sync.Map
}

func (r *forwardRoutes) matches(host string) bool {
	for i := len(host); i != -1; {
		i = strings.LastIndexByte(host[:i], '.')
		if r.hosts[host[i+1:]] {
			return true
		}
	}
	return false
}
func (p *Proxy) updateForwardRoutes() {
	routes := &forwardRoutes{hosts: map[string]bool{}}
	p.main.mu.RLock()
	for _, f := range p.main.fwdrs {
		host, _, e := net.SplitHostPort(strings.SplitN(f.Addr(), ",", 2)[0])
		if e == nil && net.ParseIP(host) == nil {
			routes.hosts[strings.ToLower(host)] = true
		}
	}
	p.main.mu.RUnlock()
	p.forwardRoutes.Store(routes)
}

// EnableDynamicMain keeps health checking active across 1-to-N node changes.
func (p *Proxy) EnableDynamicMain() { p.main.mu.Lock(); p.main.dynamic = true; p.main.mu.Unlock() }
