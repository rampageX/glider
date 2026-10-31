package subscription

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nadoo/glider/rule"
)

func testConfig(dir, raw string) Config {
	return Config{URLs: []string{raw}, Paths: []string{"direct://"}, Output: filepath.Join(dir, "nodes.conf"), CacheDir: filepath.Join(dir, "cache"), TTL: 3600, Interval: 3600, ConnectTimeout: 1, Timeout: 2, MaxBytes: 4096}
}
func manager(t *testing.T, c Config) *Manager {
	t.Helper()
	m, e := New(c, func([]string, func() error) (bool, error) { return false, nil })
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func TestCacheTTLNoSlidingAndFailedRefresh(t *testing.T) {
	var hits atomic.Int64
	var bad atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if bad.Load() {
			io.WriteString(w, ss("香港")+"\nssr://broken")
		} else {
			io.WriteString(w, ss("香港"))
		}
	}))
	defer server.Close()
	c := testConfig(t.TempDir(), server.URL+"/secret-token")
	m := manager(t, c)
	if _, hit, _, e := m.input(context.Background(), c.URLs[0]); e != nil || hit {
		t.Fatal(e)
	}
	path := filepath.Join(c.CacheDir, cacheKey(c.URLs[0])+".json")
	original, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	for i := 0; i < 3; i++ {
		if _, hit, _, e := m.input(context.Background(), c.URLs[0]); e != nil || !hit {
			t.Fatal(e)
		}
	}
	current, _ := os.ReadFile(path)
	if string(original) != string(current) || hits.Load() != 1 {
		t.Fatal("sliding cache")
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	var entry cacheEntry
	json.Unmarshal(original, &entry)
	entry.At = time.Now().Add(-2 * time.Hour)
	expired, _ := json.Marshal(entry)
	atomicWrite(path, expired)
	bad.Store(true)
	if n, _, _, e := m.input(context.Background(), c.URLs[0]); e == nil || n != nil {
		t.Fatal("stale or partial accepted")
	}
	current, _ = os.ReadFile(path)
	if string(current) != string(expired) {
		t.Fatal("failed refresh replaced cache")
	}
	bad.Store(false)
	if _, hit, _, e := m.input(context.Background(), c.URLs[0]); e != nil || hit {
		t.Fatal(e)
	}
	files, _ := os.ReadDir(c.CacheDir)
	for _, f := range files {
		if strings.Contains(f.Name(), "secret") {
			t.Fatal("URL leaked")
		}
	}
}
func TestPartialEmptyUnchangedAndPersistenceFailure(t *testing.T) {
	var body atomic.Value
	body.Store(ss("香港"))
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body.Load().(string)) }))
	defer good.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "broken") }))
	defer bad.Close()
	c := testConfig(t.TempDir(), good.URL)
	c.TTL = 0
	c.URLs = append(c.URLs, bad.URL)
	p := rule.NewProxy([]string{"direct://#name=old"}, &rule.Strategy{Strategy: "lha", Check: "disable", DialTimeout: 1}, nil)
	defer p.Close()
	m, e := New(c, p.ReplaceMain)
	if e != nil {
		t.Fatal(e)
	}
	m.Refresh(context.Background())
	defs, e := ReadSnapshot(c.Output)
	if e != nil || len(defs) != 1 {
		t.Fatal(e)
	}
	before, _ := os.Stat(c.Output)
	m.Refresh(context.Background())
	after, _ := os.Stat(c.Output)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged file overwritten")
	}
	c2 := m.conf
	c2.Matches = []string{"no-match"}
	m2, _ := New(c2, p.ReplaceMain)
	m2.Refresh(context.Background())
	if p.StatusSnapshot()[0].Forwarders[0].Name != "香港" {
		t.Fatal("empty cleared nodes")
	}
	// All failed: keep file and runtime.
	body.Store("broken")
	m.Refresh(context.Background())
	if p.StatusSnapshot()[0].Forwarders[0].Name != "香港" {
		t.Fatal("failed cleared nodes")
	}
	body.Store(ss("日本"))
	m.conf.Output = filepath.Join(c.Output, "impossible")
	m.Refresh(context.Background())
	if p.StatusSnapshot()[0].Forwarders[0].Name != "香港" {
		t.Fatal("persistence failure committed")
	}
	// A second instance filters valid shared cache without a new download.
}
func TestCrossProcessCache(t *testing.T) {
	if os.Getenv("GLIDER_CACHE_HELPER") == "1" {
		c := testConfig(os.Getenv("GLIDER_TEST_DIR"), os.Getenv("GLIDER_TEST_URL"))
		m := manager(t, c)
		if _, _, _, e := m.input(context.Background(), c.URLs[0]); e != nil {
			t.Fatal(e)
		}
		return
	}
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(100 * time.Millisecond)
		io.WriteString(w, ss("香港"))
	}))
	defer server.Close()
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCrossProcessCache$")
			cmd.Env = append(os.Environ(), "GLIDER_CACHE_HELPER=1", "GLIDER_TEST_DIR="+dir, "GLIDER_TEST_URL="+server.URL)
			if out, e := cmd.CombinedOutput(); e != nil {
				t.Errorf("helper %v %s", e, out)
			}
		}()
	}
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatalf("duplicate downloads %d", hits.Load())
	}
}

// A tiny local SOCKS5 fixture verifies remote DNS, fallback ordering and GET.
func socksFixture(t *testing.T, target string, seen *atomic.Int64, hang bool) string {
	t.Helper()
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop); ln.Close() })
	go func() {
		for {
			conn, e := ln.Accept()
			if e != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				seen.Add(1)
				if hang {
					<-stop
					return
				}
				buf := make([]byte, 512)
				if _, e := io.ReadFull(c, buf[:2]); e != nil {
					return
				}
				if _, e := io.ReadFull(c, buf[:int(buf[1])]); e != nil {
					return
				}
				c.Write([]byte{5, 0})
				if _, e := io.ReadFull(c, buf[:5]); e != nil {
					return
				}
				if buf[3] != 3 {
					return
				}
				size := int(buf[4])
				if _, e := io.ReadFull(c, buf[:size+2]); e != nil {
					return
				}
				if string(buf[:size]) != "unresolvable.invalid" {
					return
				}
				upstream, e := net.Dial("tcp", target)
				if e != nil {
					return
				}
				defer upstream.Close()
				c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 1})
				done := make(chan struct{})
				go func() { io.Copy(upstream, c); close(done) }()
				io.Copy(c, upstream)
				c.Close()
				<-done
			}(conn)
		}
	}()
	return "socks5h://" + ln.Addr().String()
}
func TestPerSubscriptionPathsAndExplicitDirect(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "*")
	var gets atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("not GET")
		}
		gets.Add(1)
		io.WriteString(w, ss("香港"))
	}))
	defer server.Close()
	var proxyHits, lastHits atomic.Int64
	proxy := socksFixture(t, strings.TrimPrefix(server.URL, "http://"), &proxyHits, false)
	last := socksFixture(t, strings.TrimPrefix(server.URL, "http://"), &lastHits, false)
	c := testConfig(t.TempDir(), server.URL)
	c.URLs = append(c.URLs, "http://unresolvable.invalid/sub")
	c.Paths = []string{"direct://#priority=30", proxy + "#priority=20", last + "#priority=10"}
	m := manager(t, c)
	for _, raw := range c.URLs {
		if _, _, _, e := m.input(context.Background(), raw); e != nil {
			t.Fatal(e)
		}
	}
	if gets.Load() != 2 || proxyHits.Load() != 1 || lastHits.Load() != 0 {
		t.Fatalf("GET %d proxy %d last %d", gets.Load(), proxyHits.Load(), lastHits.Load())
	}
}
func TestCancellationSizeAndLock(t *testing.T) {
	var hits atomic.Int64
	proxy := socksFixture(t, "", &hits, true)
	c := testConfig(t.TempDir(), "http://unresolvable.invalid/sub")
	c.Paths = []string{proxy}
	c.Timeout = 10
	m := manager(t, c)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); m.Refresh(ctx) }()
	deadline := time.Now().Add(time.Second)
	for hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not close handshake")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, strings.Repeat("x", 100)) }))
	defer server.Close()
	c = testConfig(t.TempDir(), server.URL)
	c.MaxBytes = 10
	m = manager(t, c)
	if _, _, _, e := m.input(context.Background(), server.URL); e == nil {
		t.Fatal("size limit ignored")
	}
	path := filepath.Join(t.TempDir(), "lock")
	unlock, e := lock(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if u, e := lock(ctx, path); e == nil {
		u()
		t.Fatal("lock not canceled")
	}
}

func TestSharedCacheRefilterAndRefreshSerialization(t *testing.T) {
	var hits atomic.Int64
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		io.WriteString(w, ss("香港")+"\n"+ss("日本"))
	}))
	defer server.Close()
	c := testConfig(t.TempDir(), server.URL)
	c.Matches = []string{"香港"}
	var applied atomic.Int64
	m, e := New(c, func(defs []string, save func() error) (bool, error) { applied.Add(1); return true, save() })
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	go func() { m.Refresh(context.Background()); close(done) }()
	<-started
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.Refresh(context.Background()) }()
	}
	wg.Wait()
	close(release)
	<-done
	if hits.Load() != 1 || applied.Load() != 1 {
		t.Fatal("concurrent refresh")
	}
	c.Matches = []string{"日本"}
	c.Output = filepath.Join(filepath.Dir(c.Output), "jp.conf")
	m2, e := New(c, func(defs []string, save func() error) (bool, error) {
		if len(defs) != 1 || !strings.Contains(defs[0], "%E6%97%A5%E6%9C%AC") {
			t.Error("wrong region")
		}
		return true, save()
	})
	if e != nil {
		t.Fatal(e)
	}
	m2.Refresh(context.Background())
	if hits.Load() != 1 {
		t.Fatal("filter change redownloaded")
	}
	old, e := ReadSnapshot(m.conf.Output)
	if e != nil || len(old) != 1 || !strings.Contains(old[0], "%E9%A6%99%E6%B8%AF") {
		t.Fatal("other instance modified")
	}
}

func TestPriorityTiesAndTotalHandshakeTimeout(t *testing.T) {
	paths, e := routes([]string{"direct://#priority=1", "socks5h://127.0.0.1:2#priority=5", "socks5h://127.0.0.1:3#priority=5"})
	if e != nil || len(paths) != 3 || !strings.Contains(paths[0].definition, ":2#") || !strings.Contains(paths[1].definition, ":3#") || paths[2].priority != 1 {
		t.Fatal("priority/tie order", e)
	}
	var hits atomic.Int64
	route := socksFixture(t, "", &hits, true)
	c := testConfig(t.TempDir(), "http://unresolvable.invalid/sub")
	c.Paths = []string{route}
	c.Timeout = 1
	m := manager(t, c)
	start := time.Now()
	if _, _, _, e := m.input(context.Background(), c.URLs[0]); e == nil {
		t.Fatal("hanging handshake succeeded")
	}
	if time.Since(start) > 2500*time.Millisecond {
		t.Fatal("total timeout ignored")
	}
}
