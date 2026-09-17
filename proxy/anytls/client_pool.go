package anytls

import (
	"sync"
	"time"
)

type idleClientSession struct {
	session   *session
	idleSince time.Time
}

type clientSessionPool struct {
	owner *AnyTLS

	mu   sync.Mutex
	idle []idleClientSession

	checkInterval time.Duration
	idleTimeout   time.Duration
	minIdle       int
	disableReuse  bool
	stop          chan struct{}
}

func newClientSessionPool(owner *AnyTLS, checkInterval, idleTimeout time.Duration, minIdle int, disableReuse bool) *clientSessionPool {
	if checkInterval <= 5*time.Second {
		checkInterval = 30 * time.Second
	}
	if idleTimeout <= 5*time.Second {
		idleTimeout = 30 * time.Second
	}
	if minIdle < 0 {
		minIdle = 0
	}

	p := &clientSessionPool{
		owner:         owner,
		checkInterval: checkInterval,
		idleTimeout:   idleTimeout,
		minIdle:       minIdle,
		disableReuse:  disableReuse,
		stop:          make(chan struct{}),
	}
	if !disableReuse {
		go p.cleanupLoop()
	}
	return p
}

func (p *clientSessionPool) acquire() (*session, error) {
	if !p.disableReuse {
		p.mu.Lock()
		for len(p.idle) > 0 {
			i := len(p.idle) - 1
			item := p.idle[i]
			p.idle = p.idle[:i]
			if item.session != nil && !item.session.isClosed() {
				p.mu.Unlock()
				return item.session, nil
			}
		}
		p.mu.Unlock()
	}
	return p.owner.newClientSession()
}

func (p *clientSessionPool) release(ss *session) {
	if ss == nil {
		return
	}
	if p.disableReuse || ss.isClosed() {
		_ = ss.Close()
		return
	}

	p.mu.Lock()
	p.idle = append(p.idle, idleClientSession{session: ss, idleSince: time.Now()})
	p.mu.Unlock()
}

func (p *clientSessionPool) discard(ss *session) {
	if ss != nil {
		_ = ss.Close()
	}
}

func (p *clientSessionPool) cleanupLoop() {
	ticker := time.NewTicker(p.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.cleanup()
		case <-p.stop:
			return
		}
	}
}

func (p *clientSessionPool) cleanup() {
	cutoff := time.Now().Add(-p.idleTimeout)

	p.mu.Lock()
	alive := p.idle[:0]
	for _, item := range p.idle {
		if item.session != nil && !item.session.isClosed() {
			alive = append(alive, item)
		}
	}
	p.idle = alive

	keepFloor := p.minIdle
	if keepFloor > len(p.idle) {
		keepFloor = len(p.idle)
	}
	closeList := make([]*session, 0)
	for len(p.idle) > keepFloor {
		item := p.idle[0]
		if !item.idleSince.Before(cutoff) {
			break
		}
		p.idle = p.idle[1:]
		closeList = append(closeList, item.session)
	}
	p.mu.Unlock()

	for _, ss := range closeList {
		_ = ss.Close()
	}
}
