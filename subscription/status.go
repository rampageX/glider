package subscription

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type InputStatus struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	URL            string     `json:"url"`
	Result         string     `json:"result"`
	Checking       bool       `json:"checking"`
	Source         string     `json:"source"`
	HTTPStatus     int        `json:"http_status"` // last request response; 0 for no HTTP response or a cache hit
	AttemptAt      *time.Time `json:"attempt_at,omitempty"`
	CheckedAt      *time.Time `json:"checked_at,omitempty"`
	LastDownloadAt *time.Time `json:"last_download_at,omitempty"` // last fully downloaded body, even if parsing failed
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`  // last valid parsed input
	MatchedNodes   int        `json:"matched_nodes"`
	SkippedNodes   int        `json:"skipped_nodes"`
}

const (
	resultPending        = "PENDING"
	resultSuccess        = "SUCCESS"
	resultDownloadFailed = "DOWNLOAD_FAILED"
	resultParseFailed    = "PARSE_FAILED"
	resultNoUsableNodes  = "NO_USABLE_NODES"
	resultFailed         = "FAILED" // local processing failure, not an HTTP failure
)

type Status struct {
	Inputs          []InputStatus `json:"inputs"`
	Refreshing      bool          `json:"refreshing"`
	LastAttemptAt   *time.Time    `json:"last_attempt_at,omitempty"`
	LastCompletedAt *time.Time    `json:"last_completed_at,omitempty"`
	NextRefreshAt   *time.Time    `json:"next_refresh_at,omitempty"`
	ApplyResult     string        `json:"apply_result"`
	LastAppliedAt   *time.Time    `json:"last_applied_at,omitempty"`
	Interval        int           `json:"interval"`
	RetryInterval   int           `json:"retry_interval"`
}

// Hide the entire path, query, fragment and userinfo: tokens can live in any
// of those locations. The stable ID distinguishes URLs on the same host.
func maskedURL(raw string) string {
	u, e := url.Parse(raw)
	if e != nil {
		return "[invalid URL]"
	}
	value := u.Scheme + "://" + u.Host
	if u.Path != "" && u.Path != "/" {
		value += "/…"
	} else {
		value += u.Path
	}
	if u.RawQuery != "" {
		value += "?…"
	}
	if u.Fragment != "" {
		value += "#…"
	}
	return value
}
func (m *Manager) initStatus() {
	m.status = Status{Inputs: make([]InputStatus, len(m.conf.URLs)), Interval: m.conf.Interval, RetryInterval: m.conf.RetryInterval, ApplyResult: "PENDING"}
	for i, raw := range m.conf.URLs {
		title := fmt.Sprintf("订阅 %d", i+1)
		if len(m.conf.Titles) > i && strings.TrimSpace(m.conf.Titles[i]) != "" {
			title = m.conf.Titles[i]
		}
		m.status.Inputs[i] = InputStatus{ID: cacheKey(raw)[:12], Title: title, URL: maskedURL(raw), Result: resultPending}
	}
}
func cloneTime(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}

// Snapshot never holds the refresh lock and never waits for network I/O.
func (m *Manager) Snapshot() Status {
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	s := m.status
	s.Inputs = append([]InputStatus{}, s.Inputs...)
	s.LastAttemptAt = cloneTime(s.LastAttemptAt)
	s.LastCompletedAt = cloneTime(s.LastCompletedAt)
	s.NextRefreshAt = cloneTime(s.NextRefreshAt)
	s.LastAppliedAt = cloneTime(s.LastAppliedAt)
	for i := range s.Inputs {
		s.Inputs[i].AttemptAt = cloneTime(s.Inputs[i].AttemptAt)
		s.Inputs[i].CheckedAt = cloneTime(s.Inputs[i].CheckedAt)
		s.Inputs[i].LastDownloadAt = cloneTime(s.Inputs[i].LastDownloadAt)
		s.Inputs[i].LastSuccessAt = cloneTime(s.Inputs[i].LastSuccessAt)
	}
	return s
}
func (m *Manager) beginRefresh() {
	now := time.Now()
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	m.needsRetry = false
	m.status.Refreshing = true
	m.status.LastAttemptAt = &now
	m.status.NextRefreshAt = nil
}
func (m *Manager) endRefresh() {
	now := time.Now()
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	m.status.Refreshing = false
	m.status.LastCompletedAt = &now
	for i := range m.status.Inputs {
		m.status.Inputs[i].Checking = false
	}
}
func (m *Manager) beginInput(i int) {
	now := time.Now()
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	m.status.Inputs[i].Checking = true
	m.status.Inputs[i].AttemptAt = &now
	m.status.Inputs[i].HTTPStatus = 0
}
func (m *Manager) endInput(i int, r inputResult, err error) {
	now := time.Now()
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	s := &m.status.Inputs[i]
	s.Checking = false
	s.CheckedAt = &now
	s.SkippedNodes = r.skipped
	s.HTTPStatus = r.httpStatus
	if !r.downloadAt.IsZero() {
		s.LastDownloadAt = cloneTime(&r.downloadAt)
	}
	if err != nil {
		s.Result = r.failure
		if s.Result == "" {
			s.Result = resultFailed
		}
		s.Source = ""
		if !r.downloadAt.IsZero() {
			s.Source = "download"
		}
		s.MatchedNodes = 0
		m.needsRetry = true
		return
	}
	s.MatchedNodes = len(m.conf.Filter(r.nodes))
	s.Result = resultSuccess
	if s.MatchedNodes == 0 {
		s.Result = resultNoUsableNodes
	}
	s.Source = "download"
	if r.hit {
		s.Source = "cache"
	}
	s.LastSuccessAt = cloneTime(&r.at)
}
func (m *Manager) setApply(result string, retry bool) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	m.status.ApplyResult = result
	if result == "UPDATED" {
		now := time.Now()
		m.status.LastAppliedAt = &now
	}
	m.needsRetry = m.needsRetry || retry
}
func (m *Manager) nextDelay() time.Duration {
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	seconds := m.conf.Interval
	if m.needsRetry && m.conf.RetryInterval > 0 {
		seconds = m.conf.RetryInterval
	}
	return time.Duration(seconds) * time.Second
}
func (m *Manager) setNext(t *time.Time) {
	m.statusMu.Lock()
	m.status.NextRefreshAt = t
	m.statusMu.Unlock()
}
