package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInputStatusCacheFailureAndSafeURL(t *testing.T) {
	var hits atomic.Int64
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if fail.Load() {
			http.Error(w, "failed", 503)
			return
		}
		io.WriteString(w, ss("美国"))
	}))
	defer server.Close()
	c := testConfig(t.TempDir(), server.URL+"/SECRET_PATH?token=SECRET_QUERY#SECRET_FRAGMENT")
	c.Titles = []string{"我的订阅"}
	c.RetryInterval = 7200
	c.Matches = []string{"美国"}
	m := manager(t, c)
	before := m.Snapshot()
	if before.Inputs[0].Result != "PENDING" || before.Inputs[0].Title != "我的订阅" {
		t.Fatal(before)
	}
	serialized, _ := json.Marshal(before)
	if strings.Contains(string(serialized), "SECRET") {
		t.Fatal("URL secret exposed")
	}
	m.Refresh(context.Background())
	first := m.Snapshot().Inputs[0]
	if first.Result != "SUCCESS" || first.Source != "download" || first.LastSuccessAt == nil || first.CheckedAt == nil || first.MatchedNodes != 1 {
		t.Fatal(first)
	}
	m.Refresh(context.Background())
	cached := m.Snapshot().Inputs[0]
	if hits.Load() != 1 || cached.Source != "cache" || !first.LastSuccessAt.Equal(*cached.LastSuccessAt) {
		t.Fatal("cache masquerades as new download")
	}
	fail.Store(true)
	m.conf.TTL = 0
	m.Refresh(context.Background())
	failed := m.Snapshot()
	if failed.Inputs[0].Result != "FAILED" || !failed.Inputs[0].LastSuccessAt.Equal(*first.LastSuccessAt) || m.nextDelay() != 2*time.Hour {
		t.Fatal(failed)
	}
	if failed.ApplyResult != "RETAINED_NO_INPUT" {
		t.Fatal(failed.ApplyResult)
	}
	// Returned snapshots must not expose mutable backing arrays or timestamps.
	failed.Inputs[0].Title = "changed"
	*failed.Inputs[0].LastSuccessAt = time.Time{}
	actual := m.Snapshot().Inputs[0]
	if actual.Title != "我的订阅" || actual.LastSuccessAt.IsZero() {
		t.Fatal("snapshot aliases live state")
	}
}
func TestRetryRunCancellationAndNoConcurrentRefresh(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			http.Error(w, "bad", 503)
			return
		}
		io.WriteString(w, ss("美国"))
	}))
	defer server.Close()
	c := testConfig(t.TempDir(), server.URL)
	c.RetryInterval = 1
	m := manager(t, c)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); m.Run(ctx, nil) }()
	deadline := time.Now().Add(4 * time.Second)
	sawRetry := false
	for time.Now().Before(deadline) {
		s := m.Snapshot()
		if s.NextRefreshAt != nil && s.Inputs[0].Result == "FAILED" {
			sawRetry = true
		}
		if hits.Load() >= 2 && !s.Refreshing && s.Inputs[0].Result == "SUCCESS" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !sawRetry || hits.Load() != 2 || m.nextDelay() != time.Hour {
		t.Fatalf("retry/recovery %v %d", sawRetry, hits.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timer did not cancel")
	}
	if m.Snapshot().NextRefreshAt != nil {
		t.Fatal("stale next refresh after stop")
	}
	m.conf.RetryInterval = 0
	m.statusMu.Lock()
	m.needsRetry = true
	m.statusMu.Unlock()
	if m.nextDelay() != time.Hour {
		t.Fatal("retry disabled ignored")
	}
}
func TestStatusNonblockingDuringDownloadAndPartialCacheReuse(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
		http.Error(w, "failed", 503)
	}))
	defer slow.Close()
	var hits atomic.Int64
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); io.WriteString(w, ss("美国")) }))
	defer good.Close()
	c := testConfig(t.TempDir(), good.URL)
	c.URLs = append(c.URLs, slow.URL)
	c.Titles = []string{"好订阅", "坏订阅"}
	c.RetryInterval = 7200
	m := manager(t, c)
	done := make(chan struct{})
	go func() { m.Refresh(context.Background()); close(done) }()
	<-started
	read := make(chan Status, 1)
	go func() { read <- m.Snapshot() }()
	select {
	case s := <-read:
		if !s.Refreshing || !s.Inputs[1].Checking {
			t.Error("no live progress")
		}
	case <-time.After(250 * time.Millisecond):
		t.Error("status blocked by network")
	}
	close(release)
	<-done
	// Subsequent partial-failure retry reuses the good input's valid cache.
	m.Refresh(context.Background())
	s := m.Snapshot()
	if hits.Load() != 1 || s.Inputs[0].Source != "cache" || s.Inputs[1].Result != "FAILED" || m.nextDelay() != 2*time.Hour {
		t.Fatal(s)
	}
}
func TestTitlesValidationAndPersistenceOutcome(t *testing.T) {
	c := testConfig(t.TempDir(), "https://example.invalid/sub")
	c.Titles = []string{"one", "two"}
	if _, e := New(c, nil); e == nil {
		t.Fatal("misaligned titles accepted")
	}
	c.Titles = nil
	if e := c.Resolve(t.TempDir()); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, ss("美国")) }))
	defer server.Close()
	c.URLs = []string{server.URL}
	c.RetryInterval = 7200
	m, e := New(c, func([]string, func() error) (bool, error) { return false, errors.New("disk failed") })
	if e != nil {
		t.Fatal(e)
	}
	m.Refresh(context.Background())
	s := m.Snapshot()
	if s.Inputs[0].Result != "SUCCESS" || s.ApplyResult != "PERSIST_FAILED" || m.nextDelay() != 2*time.Hour {
		t.Fatal(s)
	}
}
func TestManualTriggerDoesNotWaitForNormalInterval(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); io.WriteString(w, ss("美国")) }))
	defer server.Close()
	c := testConfig(t.TempDir(), server.URL)
	c.TTL = 0
	m := manager(t, c)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	triggers := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() { defer close(done); m.Run(ctx, triggers) }()
	deadline := time.Now().Add(2 * time.Second)
	for m.Snapshot().NextRefreshAt == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	triggers <- os.Interrupt
	for hits.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if hits.Load() != 2 {
		t.Fatal("manual trigger ignored")
	}
}
