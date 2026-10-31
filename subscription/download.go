package subscription

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/rule"
)

type route struct {
	definition string
	priority   int
	intface    string
}

func routes(paths []string) ([]route, error) {
	var out []route
	for _, s := range paths {
		u, e := url.Parse(s)
		if e != nil {
			return nil, errors.New("invalid subscription-proxy")
		}
		// Reuse Glider forwarder options and chain URL syntax, including priority.
		f, e := rule.ForwarderFromURL(s, "", time.Second, time.Second)
		if e != nil {
			return nil, errors.New("invalid subscription-proxy")
		}
		if u.Scheme == "" {
			return nil, errors.New("invalid subscription-proxy")
		}
		opts, _ := url.ParseQuery(u.Fragment)
		out = append(out, route{definition: s, priority: int(f.Priority()), intface: opts.Get("interface")})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].priority > out[j].priority })
	if len(out) == 0 {
		return nil, errors.New("no download routes")
	}
	return out, nil
}

// requestBase supplies deadlines to every proxy handshake through the existing
// Glider dialer chain. Cancellation closes the underlying socket immediately.
type requestBase struct {
	proxy.Dialer
	ctx     context.Context
	timeout time.Duration
}
type requestConn struct {
	net.Conn
	stop func() bool
	once sync.Once
}

func (c *requestConn) Close() error { c.once.Do(func() { c.stop() }); return c.Conn.Close() }
func (d *requestBase) Dial(network, addr string) (net.Conn, error) {
	if e := d.ctx.Err(); e != nil {
		return nil, e
	}
	var c net.Conn
	var e error
	if dc, ok := d.Dialer.(interface {
		DialContext(context.Context, string, string) (net.Conn, error)
	}); ok {
		c, e = dc.DialContext(d.ctx, network, addr)
	} else {
		c, e = d.Dialer.Dial(network, addr)
	}
	if e != nil {
		return nil, e
	}
	deadline := time.Now().Add(d.timeout)
	if t, ok := d.ctx.Deadline(); ok && t.Before(deadline) {
		deadline = t
	}
	c.SetDeadline(deadline)
	stop := context.AfterFunc(d.ctx, func() { c.Close() })
	if e = d.ctx.Err(); e != nil {
		stop()
		c.Close()
		return nil, e
	}
	return &requestConn{Conn: c, stop: stop}, nil
}
func download(ctx context.Context, raw string, r route, c Config) ([]byte, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("invalid subscription URL")
	}
	ctx, cancel := context.WithTimeout(ctx, c.duration(c.Timeout))
	defer cancel()
	base, e := proxy.NewDirect(r.intface, c.duration(c.ConnectTimeout), 0)
	if e != nil {
		return nil, errors.New("stage=dial reason=invalid-interface")
	}
	// Build with a bounded base rather than the runtime Proxy: download requests
	// never recursively invoke subscription refresh or acquire its cache lock.
	var d proxy.Dialer = &requestBase{Dialer: base, ctx: ctx, timeout: c.duration(c.Timeout)}
	// ForwarderFromURL supplies parsing/validation; instantiate the same chain on
	// the request base so SOCKS/HTTP handshakes receive cancellation and deadlines.
	chain, _, _ := strings.Cut(r.definition, "#")
	for _, part := range strings.Split(chain, ",") {
		d, e = proxy.DialerFromURL(part, d)
		if e != nil {
			return nil, errors.New("invalid download route")
		}
	}
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSHandshakeTimeout: c.duration(c.ConnectTimeout), ResponseHeaderTimeout: c.duration(c.Timeout), DialContext: func(_ context.Context, network, addr string) (net.Conn, error) {
		debugf(c, "input=%s route=%s stage=proxy-dial-start", cacheKey(raw)[:8], routeLabel(r.definition))
		conn, err := d.Dial(network, addr)
		if err != nil {
			debugf(c, "input=%s route=%s stage=proxy-dial-failed reason=%s", cacheKey(raw)[:8], routeLabel(r.definition), networkReason(err))
		} else {
			debugf(c, "input=%s route=%s stage=proxy-dial-ok", cacheKey(raw)[:8], routeLabel(r.definition))
		}
		return conn, err
	}}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: c.duration(c.Timeout), CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("redirect limit")
		}
		if req.URL.Scheme != "https" && req.URL.Scheme != "http" {
			return errors.New("invalid redirect")
		}
		return nil
	}}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return nil, errors.New("invalid request")
	}
	trace := &httptrace.ClientTrace{
		TLSHandshakeStart: func() { debugf(c, "input=%s route=%s stage=tls-start", cacheKey(raw)[:8], routeLabel(r.definition)) },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err != nil {
				debugf(c, "input=%s route=%s stage=tls-failed reason=%s", cacheKey(raw)[:8], routeLabel(r.definition), networkReason(err))
			} else {
				debugf(c, "input=%s route=%s stage=tls-ok", cacheKey(raw)[:8], routeLabel(r.definition))
			}
		},
		GotFirstResponseByte: func() {
			debugf(c, "input=%s route=%s stage=response-first-byte", cacheKey(raw)[:8], routeLabel(r.definition))
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	req.Header.Set("User-Agent", "Glider-subscription/1")
	resp, e := client.Do(req)
	if e != nil {
		return nil, fmt.Errorf("stage=get reason=%s", networkReason(e))
	}
	defer resp.Body.Close()
	debugf(c, "input=%s route=%s stage=response status=%d media=%s declared-bytes=%d", cacheKey(raw)[:8], routeLabel(r.definition), resp.StatusCode, mediaKind(resp.Header.Get("Content-Type")), resp.ContentLength)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stage=response status=%d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, int64(c.MaxBytes)+1))
	if e != nil {
		return nil, fmt.Errorf("stage=read reason=%s", networkReason(e))
	}
	if len(b) > c.MaxBytes {
		return nil, errors.New("response limit exceeded")
	}
	debugf(c, "input=%s route=%s stage=body %s", cacheKey(raw)[:8], routeLabel(r.definition), bodySummary(b))
	return b, nil
}
