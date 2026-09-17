package rule

import (
	"errors"
	"hash/fnv"
	"net"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// forwarder slice orderd by priority.
type priSlice []*Forwarder

func (p priSlice) Len() int           { return len(p) }
func (p priSlice) Less(i, j int) bool { return p[i].Priority() > p[j].Priority() }
func (p priSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// FwdrGroup is a forwarder group.
type FwdrGroup struct {
	name       string
	config     *Strategy
	fwdrs      priSlice
	avail      []*Forwarder // available forwarders
	mu         sync.RWMutex
	selectionMu sync.RWMutex
	current    *Forwarder
	index      uint32
	priority   uint32
	next       func(addr string) *Forwarder
}

// NewFwdrGroup returns a new forward group.
func NewFwdrGroup(rulePath string, s []string, c *Strategy) *FwdrGroup {
	var fwdrs []*Forwarder
	for _, chain := range s {
		fwdr, err := ForwarderFromURL(chain, c.IntFace,
			time.Duration(c.DialTimeout)*time.Second, time.Duration(c.RelayTimeout)*time.Second)
		if err != nil {
			log.Fatal(err)
		}
		fwdr.SetMaxFailures(uint32(c.MaxFailures))
		fwdrs = append(fwdrs, fwdr)
	}

	if len(fwdrs) == 0 {
		direct, err := DirectForwarder(c.IntFace,
			time.Duration(c.DialTimeout)*time.Second, time.Duration(c.RelayTimeout)*time.Second)
		if err != nil {
			log.Fatal(err)
		}
		fwdrs = append(fwdrs, direct)
		c.Strategy = "rr"
	}

	name := strings.TrimSuffix(filepath.Base(rulePath), filepath.Ext(rulePath))
	return newFwdrGroup(name, fwdrs, c)
}

func newFwdrGroup(name string, fwdrs []*Forwarder, c *Strategy) *FwdrGroup {
	p := &FwdrGroup{name: name, fwdrs: fwdrs, config: c}
	sort.Sort(p.fwdrs)
	p.init()
	p.next = p.scheduleRR

	if count := len(fwdrs); count > 1 {
		switch c.Strategy {
		case "rr":
			p.next = p.scheduleRR
			log.F("[strategy] %s: %d forwarders forward in round robin mode.", name, count)
		case "ha":
			p.next = p.scheduleHA
			log.F("[strategy] %s: %d forwarders forward in high availability mode.", name, count)
		case "lha":
			p.next = p.scheduleLHA
			log.F("[strategy] %s: %d forwarders forward in latency based high availability mode.", name, count)
		case "dh":
			p.next = p.scheduleDH
			log.F("[strategy] %s: %d forwarders forward in destination hashing mode.", name, count)
		default:
			p.next = p.scheduleRR
			log.F("[strategy] %s: not supported forward mode '%s', use round robin mode for %d forwarders.", name, c.Strategy, count)
		}
	}

	for _, f := range fwdrs {
		f.AddHandler(p.onStatusChanged)
	}
	return p
}

func (p *FwdrGroup) Dial(network, addr string) (net.Conn, proxy.Dialer, error) {
	nd := p.NextDialer(addr)
	c, err := nd.Dial(network, addr)
	return c, nd, err
}

func (p *FwdrGroup) DialUDP(network, addr string) (pc net.PacketConn, dialer proxy.UDPDialer, err error) {
	nd := p.NextDialer(addr)
	pc, err = nd.DialUDP(network, addr)
	return pc, nd, err
}

// NextDialer returns the next dialer.
func (p *FwdrGroup) NextDialer(dstAddr string) proxy.Dialer {
	p.mu.RLock()
	var fwdr *Forwarder
	if len(p.avail) == 0 {
		fwdr = p.fwdrs[atomic.AddUint32(&p.index, 1)%uint32(len(p.fwdrs))]
	} else {
		fwdr = p.next(dstAddr)
	}
	p.mu.RUnlock()

	if p.config.Strategy == "lha" {
		p.recordSelection(fwdr)
	}
	return fwdr
}

// recordSelection stores the last forwarder actually selected by LHA and logs only changes.
func (p *FwdrGroup) recordSelection(fwdr *Forwarder) {
	if fwdr == nil {
		return
	}
	p.selectionMu.Lock()
	defer p.selectionMu.Unlock()
	old := p.current
	if old == fwdr {
		return
	}
	p.current = fwdr
	if old == nil {
		log.F("[lha] %s: selected %s (%dms)", p.name, fwdr.Name(), time.Duration(fwdr.Latency()).Milliseconds())
		return
	}
	log.F("[lha] %s: switch %s (%dms) -> %s (%dms)", p.name,
		old.Name(), time.Duration(old.Latency()).Milliseconds(),
		fwdr.Name(), time.Duration(fwdr.Latency()).Milliseconds())
}

// CurrentForwarder returns the last forwarder actually selected by LHA.
func (p *FwdrGroup) CurrentForwarder() *Forwarder {
	p.selectionMu.RLock()
	defer p.selectionMu.RUnlock()
	return p.current
}

func (p *FwdrGroup) Priority() uint32 { return atomic.LoadUint32(&p.priority) }
func (p *FwdrGroup) SetPriority(pri uint32) { atomic.StoreUint32(&p.priority, pri) }

func (p *FwdrGroup) init() {
	for _, f := range p.fwdrs {
		if f.Enabled() {
			p.SetPriority(f.Priority())
			break
		}
	}
	p.avail = nil
	for _, f := range p.fwdrs {
		if f.Enabled() && f.Priority() >= p.Priority() {
			p.avail = append(p.avail, f)
		}
	}
	if len(p.avail) == 0 {
		p.SetPriority(0)
	}
}

func (p *FwdrGroup) onStatusChanged(fwdr *Forwarder) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if fwdr.Enabled() {
		if fwdr.Priority() == p.Priority() {
			p.avail = append(p.avail, fwdr)
		} else if fwdr.Priority() > p.Priority() {
			p.init()
		}
		log.F("[group] %s: %s(%d) changed status from DISABLED to ENABLED (%d of %d currently enabled)",
			p.name, fwdr.Name(), fwdr.Priority(), len(p.avail), len(p.fwdrs))
	} else {
		for i, f := range p.avail {
			if f == fwdr {
				p.avail[i], p.avail = p.avail[len(p.avail)-1], p.avail[:len(p.avail)-1]
				break
			}
		}
		log.F("[group] %s: %s(%d) changed status from ENABLED to DISABLED (%d of %d currently enabled)",
			p.name, fwdr.Name(), fwdr.Priority(), len(p.avail), len(p.fwdrs))
	}
	if len(p.avail) == 0 {
		p.init()
	}
}

func (p *FwdrGroup) Check() {
	if len(p.fwdrs) == 1 {
		log.F("[group] %s: only 1 forwarder found, disable health checking", p.name)
		return
	}
	if !strings.Contains(p.config.Check, "://") {
		p.config.Check += "://"
	}
	u, err := url.Parse(p.config.Check)
	if err != nil {
		log.F("[group] %s: parse check config error: %s, disable health checking", p.name, err)
		return
	}
	addr := u.Host
	timeout := time.Duration(p.config.CheckTimeout) * time.Second

	var checker Checker
	switch u.Scheme {
	case "tcp":
		checker = newTcpChecker(addr, timeout)
	case "http", "https":
		expect := "HTTP"
		params, _ := url.ParseQuery(u.Fragment)
		if ex := params.Get("expect"); ex != "" {
			expect = ex
		}
		checker = newHttpChecker(addr, u.RequestURI(), expect, timeout, u.Scheme == "https")
	case "file":
		checker = newFileChecker(u.Host + u.Path)
	default:
		log.F("[group] %s: unknown scheme in check config `%s`, disable health checking", p.name, p.config.Check)
		return
	}
	log.F("[group] %s: using check config: %s", p.name, p.config.Check)
	for i := range p.fwdrs {
		go p.check(p.fwdrs[i], checker)
	}
}

func (p *FwdrGroup) check(fwdr *Forwarder, checker Checker) {
	wait := uint8(0)
	intval := time.Duration(p.config.CheckInterval) * time.Second
	for {
		time.Sleep(intval * time.Duration(wait))
		if wait > 0 && (fwdr.Priority() < p.Priority()) {
			continue
		}
		if fwdr.Enabled() && p.config.CheckDisabledOnly {
			continue
		}
		elapsed, err := checker.Check(fwdr)
		if err != nil {
			if errors.Is(err, proxy.ErrNotSupported) {
				fwdr.SetMaxFailures(0)
				log.F("[check] %s: %s(%d), %s, stop checking", p.name, fwdr.Name(), fwdr.Priority(), err)
				fwdr.Enable()
				break
			}
			wait++
			if wait > 16 {
				wait = 16
			}
			log.F("[check] %s: %s(%d), FAILED. error: %s", p.name, fwdr.Name(), fwdr.Priority(), err)
			fwdr.Disable()
			continue
		}
		wait = 1
		p.setLatency(fwdr, elapsed)
		log.F("[check] %s: %s(%d), SUCCESS. Elapsed: %dms, Latency: %dms.",
			p.name, fwdr.Name(), fwdr.Priority(), elapsed.Milliseconds(), time.Duration(fwdr.Latency()).Milliseconds())
		fwdr.Enable()
	}
}

func (p *FwdrGroup) setLatency(fwdr *Forwarder, elapsed time.Duration) {
	newLatency := int64(elapsed)
	if cnt := p.config.CheckLatencySamples; cnt > 1 {
		if lastLatency := fwdr.Latency(); lastLatency > 0 {
			newLatency = (lastLatency*(int64(cnt)-1) + int64(elapsed)) / int64(cnt)
		}
	}
	fwdr.SetLatency(newLatency)
}

func (p *FwdrGroup) scheduleRR(dstAddr string) *Forwarder {
	return p.avail[atomic.AddUint32(&p.index, 1)%uint32(len(p.avail))]
}

func (p *FwdrGroup) scheduleHA(dstAddr string) *Forwarder {
	return p.avail[0]
}

// Latency based High Availability. Scheduling behavior is intentionally unchanged.
func (p *FwdrGroup) scheduleLHA(dstAddr string) *Forwarder {
	oldfwdr, newfwdr := p.avail[0], p.avail[0]
	lowest := oldfwdr.Latency()
	for _, f := range p.avail {
		if f.Latency() < lowest {
			lowest = f.Latency()
			newfwdr = f
		}
	}
	tolerance := int64(p.config.CheckTolerance) * int64(time.Millisecond)
	if newfwdr.Latency() < (oldfwdr.Latency() - tolerance) {
		return newfwdr
	}
	return oldfwdr
}

func (p *FwdrGroup) scheduleDH(dstAddr string) *Forwarder {
	fnv1a := fnv.New32a()
	fnv1a.Write([]byte(dstAddr))
	return p.avail[fnv1a.Sum32()%uint32(len(p.avail))]
}
