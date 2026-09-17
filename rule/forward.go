package rule

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

type StatusHandler func(*Forwarder)

type Forwarder struct {
	proxy.Dialer
	url         string
	addr        string
	name        string
	priority    uint32
	maxFailures uint32
	disabled    uint32
	failures    uint32
	latency     int64
	intface     string
	handlers    []StatusHandler
}

func ForwarderFromURL(s, intface string, dialTimeout, relayTimeout time.Duration) (f *Forwarder, err error) {
	f = &Forwarder{url: s}
	ss := strings.Split(s, "#")
	if len(ss) > 1 {
		err = f.parseOption(ss[1])
	}
	iface := intface
	if f.intface != "" && f.intface != intface {
		iface = f.intface
	}
	var d proxy.Dialer
	d, err = proxy.NewDirect(iface, dialTimeout, relayTimeout)
	if err != nil { return nil, err }
	var addrs []string
	for _, u := range strings.Split(ss[0], ",") {
		d, err = proxy.DialerFromURL(u, d)
		if err != nil { return nil, err }
		cnt := len(addrs)
		if cnt == 0 || (cnt > 0 && d.Addr() != addrs[cnt-1]) { addrs = append(addrs, d.Addr()) }
	}
	f.Dialer = d
	f.addr = d.Addr()
	if len(addrs) > 0 { f.addr = strings.Join(addrs, ",") }
	f.Disable()
	return f, err
}

func DirectForwarder(intface string, dialTimeout, relayTimeout time.Duration) (*Forwarder, error) {
	d, err := proxy.NewDirect(intface, dialTimeout, relayTimeout)
	if err != nil { return nil, err }
	return &Forwarder{Dialer: d, addr: d.Addr()}, nil
}

func (f *Forwarder) parseOption(option string) error {
	query, err := url.ParseQuery(option)
	if err != nil { return err }
	var priority uint64
	p := query.Get("priority")
	if p != "" { priority, err = strconv.ParseUint(p, 10, 32) }
	f.SetPriority(uint32(priority))
	f.intface = query.Get("interface")
	f.name = strings.Trim(strings.TrimSpace(query.Get("name")), "\"'")
	return err
}

func (f *Forwarder) Addr() string { return f.addr }
func (f *Forwarder) Name() string { if f.name != "" { return f.name }; return f.addr }
func (f *Forwarder) URL() string { return f.url }

func (f *Forwarder) Dial(network, addr string) (c net.Conn, err error) {
	c, err = f.Dialer.Dial(network, addr)
	if err != nil { f.IncFailures() }
	return c, err
}

func (f *Forwarder) Failures() uint32 { return atomic.LoadUint32(&f.failures) }
func (f *Forwarder) IncFailures() {
	failures := atomic.AddUint32(&f.failures, 1)
	if f.MaxFailures() == 0 { return }
	if failures == f.MaxFailures() && f.Enabled() {
		log.F("[forwarder] %s(%d) reaches maxfailures: %d", f.Name(), f.Priority(), f.MaxFailures())
		f.Disable()
	}
}
func (f *Forwarder) AddHandler(h StatusHandler) { f.handlers = append(f.handlers, h) }
func (f *Forwarder) Enable() {
	if atomic.CompareAndSwapUint32(&f.disabled, 1, 0) { for _, h := range f.handlers { h(f) } }
	atomic.StoreUint32(&f.failures, 0)
}
func (f *Forwarder) Disable() {
	if atomic.CompareAndSwapUint32(&f.disabled, 0, 1) { for _, h := range f.handlers { h(f) } }
}
func (f *Forwarder) Enabled() bool { return !isTrue(atomic.LoadUint32(&f.disabled)) }
func isTrue(n uint32) bool { return n&1 == 1 }
func (f *Forwarder) Priority() uint32 { return atomic.LoadUint32(&f.priority) }
func (f *Forwarder) SetPriority(l uint32) { atomic.StoreUint32(&f.priority, l) }
func (f *Forwarder) MaxFailures() uint32 { return atomic.LoadUint32(&f.maxFailures) }
func (f *Forwarder) SetMaxFailures(l uint32) { atomic.StoreUint32(&f.maxFailures, l) }
func (f *Forwarder) Latency() int64 { return atomic.LoadInt64(&f.latency) }
func (f *Forwarder) SetLatency(l int64) { atomic.StoreInt64(&f.latency, l) }
