package rule

import (
	"context"
	"errors"
	_ "github.com/nadoo/glider/proxy/reject"
	_ "github.com/nadoo/glider/proxy/tcp"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testStrategy() *Strategy {
	return &Strategy{Strategy: "lha", Check: "disable", DialTimeout: 1, CheckTimeout: 1, CheckInterval: 1, MaxFailures: 3}
}
func TestReplaceTransactionIdentityAndOrder(t *testing.T) {
	p := NewProxy([]string{"direct://#name=a", "reject://#name=b"}, testStrategy(), nil)
	defer p.Close()
	a := p.main.fwdrs[0]
	a.Enable()
	a.SetLatency(123)
	a.IncFailures()
	called := false
	if changed, e := p.ReplaceMain([]string{a.URL(), p.main.fwdrs[1].URL()}, func() error { called = true; return nil }); changed || e != nil || called {
		t.Fatal("unchanged committed")
	}
	if _, e := p.ReplaceMain([]string{"direct://#name=c"}, func() error { return errors.New("disk full") }); e == nil {
		t.Fatal("expected disk failure")
	}
	if p.main.fwdrs[0] != a {
		t.Fatal("rollback lost identity")
	}
	if _, e := p.ReplaceMain([]string{"bad-scheme://"}, func() error { t.Fatal("invalid persisted"); return nil }); e == nil {
		t.Fatal("invalid accepted")
	}
	if _, e := p.ReplaceMain([]string{a.URL(), a.URL(), "direct://#name=c"}, func() error { return nil }); e != nil {
		t.Fatal(e)
	}
	if p.main.fwdrs[0] != a || a.Latency() != 123 || a.Failures() != 1 || !a.Enabled() {
		t.Fatal("unchanged state lost")
	}
	if len(p.StatusSnapshot()[0].Forwarders) != 3 {
		t.Fatal("stale status")
	}
	// Old connections can still report success; retired forwarders cannot rejoin.
	old := p.main.fwdrs[2]
	p.ReplaceMain([]string{a.URL()}, nil)
	old.Enable()
	if len(p.main.avail) != 1 {
		t.Fatal("removed handler rejoined")
	}
}
func TestConnectionContinuesAfterReplace(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	p := NewProxy([]string{"tcp://" + ln.Addr().String() + "#name=old"}, testStrategy(), nil)
	defer p.Close()
	conn, _, e := p.Dial("tcp", ln.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	p.ReplaceMain([]string{"reject://#name=new"}, nil)
	conn.Write([]byte("alive"))
	buf := make([]byte, 5)
	if _, e = io.ReadFull(conn, buf); e != nil || string(buf) != "alive" {
		t.Fatal("existing connection lost", e)
	}
	if c, _, e := p.Dial("tcp", ln.Addr().String()); e == nil {
		c.Close()
		t.Fatal("new connection used old node")
	}
}

type countingChecker struct{ count atomic.Int64 }

func (c *countingChecker) Check(*Forwarder) (time.Duration, error) {
	c.count.Add(1)
	return time.Millisecond, nil
}
func TestHealthExitAndConcurrentSnapshots(t *testing.T) {
	p := NewProxy([]string{"direct://#name=a", "direct://#name=b"}, testStrategy(), nil)
	checker := &countingChecker{}
	p.main.mu.Lock()
	p.main.checker = checker
	p.main.checks = map[*Forwarder]context.CancelFunc{}
	for _, f := range p.main.fwdrs {
		p.main.startCheckLocked(f)
	}
	p.main.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for checker.count.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if checker.count.Load() < 2 {
		t.Fatal("no checks")
	}
	var wg sync.WaitGroup
	for j := 0; j < 3; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				p.StatusSnapshot()
				p.NextDialer("x:1")
			}
		}()
	}
	for i := 0; i < 30; i++ {
		p.ReplaceMain([]string{"direct://#name=c", "direct://#name=a"}, nil)
		p.ReplaceMain([]string{"direct://#name=a"}, nil)
	}
	wg.Wait()
	start := time.Now()
	closed := make(chan struct{})
	go func() { p.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("health tasks did not exit")
	}
	if time.Since(start) > time.Second {
		t.Fatal("check sleep not canceled")
	}
	count := checker.count.Load()
	time.Sleep(30 * time.Millisecond)
	if checker.count.Load() != count {
		t.Fatal("checks survived close")
	}
}

func TestLegacyAndEncodedNames(t *testing.T) {
	for _, tc := range []struct{ value, want string }{{`"legacy"`, "legacy"}, {"%20%27literal%27%20", " 'literal' "}} {
		f, e := ForwarderFromURL("direct://#name="+tc.value, "", time.Second, time.Second)
		if e != nil || f.Name() != tc.want {
			t.Fatalf("legacy/encoded name %q %v", f.Name(), e)
		}
	}
}
