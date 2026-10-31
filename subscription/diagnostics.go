package subscription

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/nadoo/glider/pkg/log"
)

// Only fixed labels and numeric metadata may enter diagnostics. In particular,
// never format an arbitrary parser, HTTP, URL or proxy error with %v.
type parseStageError struct {
	stage  string
	reason string
}

func (e *parseStageError) Error() string { return e.stage + ":" + e.reason }
func atStage(stage string, err error) error {
	return &parseStageError{stage: stage, reason: parseReason(err)}
}
func parseReason(err error) string {
	var staged *parseStageError
	if errors.As(err, &staged) {
		return staged.Error()
	}
	var offset base64.CorruptInputError
	if errors.As(err, &offset) {
		return fmt.Sprintf("invalid-base64-offset-%d", int64(offset))
	}
	var escape url.EscapeError
	if errors.As(err, &escape) {
		return "invalid-percent-escape"
	}
	var number *strconv.NumError
	if errors.As(err, &number) {
		return "invalid-number"
	}
	if err == nil {
		return "none"
	}
	switch err.Error() {
	case "invalid base64":
		return "invalid-base64"
	case "invalid endpoint":
		return "invalid-endpoint"
	case "invalid port":
		return "invalid-port"
	case "invalid host":
		return "invalid-host"
	case "missing endpoint":
		return "missing-endpoint"
	case "missing credentials":
		return "missing-credentials"
	case "invalid SSR":
		return "invalid-ssr-fields"
	case "unexpected subscription line":
		return "unexpected-line"
	}
	return "invalid-node"
}
func nodeProtocol(line string) string {
	if strings.HasPrefix(line, "ss://") {
		return "ss"
	}
	if strings.HasPrefix(line, "ssr://") {
		return "ssr"
	}
	return "other"
}
func bodyKind(body []byte) string {
	s := strings.TrimSpace(strings.TrimPrefix(string(body), "\ufeff"))
	if s == "" {
		return "empty"
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") {
		return "html"
	}
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return "json-like"
	}
	if strings.HasPrefix(s, "ss://") {
		return "ss-lines"
	}
	if strings.HasPrefix(s, "ssr://") {
		return "ssr-lines"
	}
	if _, e := decode(s); e == nil {
		return "base64"
	}
	if looksPlain(s) {
		return "mixed-lines"
	}
	return "other-text"
}
func bodySummary(body []byte) string {
	kind := bodyKind(body)
	s := strings.TrimSpace(strings.TrimPrefix(string(body), "\ufeff"))
	if kind == "base64" {
		s, _ = decode(s)
	}
	ss, ssr, other := 0, 0, 0
	classes := map[string]int{}
	for _, line := range strings.Split(s, "\n") {
		line = cleanLine(line)
		if line == "" {
			continue
		}
		classes[lineClass(line)]++
		switch nodeProtocol(line) {
		case "ss":
			ss++
		case "ssr":
			ssr++
		default:
			other++
		}
	}
	var labels []string
	for label, count := range classes {
		labels = append(labels, fmt.Sprintf("%s:%d", label, count))
	}
	sort.Strings(labels)
	return fmt.Sprintf("body=%s bytes=%d lines-ss=%d lines-ssr=%d lines-other=%d classes=%s", kind, len(body), ss, ssr, other, strings.Join(labels, ","))
}
func networkReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns-failed"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connection-refused"
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return "connection-reset"
	}
	if errors.Is(err, io.EOF) {
		return "eof-before-response"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "truncated-stream"
	}
	if errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) {
		return "network-unreachable"
	}
	var record tls.RecordHeaderError
	if errors.As(err, &record) {
		return "tls-invalid-record"
	}
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &unknown) {
		return "tls-untrusted-certificate"
	}
	var host x509.HostnameError
	if errors.As(err, &host) {
		return "tls-hostname-mismatch"
	}
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) {
		return "tls-invalid-certificate"
	}
	return "network-proxy-or-tls-failed"
}
func routeLabel(def string) string {
	chain, _, _ := strings.Cut(def, "#")
	var labels []string
	for _, part := range strings.Split(chain, ",") {
		u, e := url.Parse(part)
		if e != nil {
			labels = append(labels, "invalid")
			continue
		}
		scheme := "proxy"
		switch u.Scheme {
		case "direct", "socks5", "socks5h", "http", "https", "tls":
			scheme = u.Scheme
		}
		// Proxy credentials, domain names and options are never included.
		if net.ParseIP(u.Hostname()) != nil {
			labels = append(labels, scheme+"://"+net.JoinHostPort(u.Hostname(), u.Port()))
		} else {
			labels = append(labels, scheme)
		}
	}
	return strings.Join(labels, ",")
}
func mediaKind(header string) string {
	media, _, _ := strings.Cut(strings.ToLower(header), ";")
	switch strings.TrimSpace(media) {
	case "text/plain":
		return "text/plain"
	case "text/html":
		return "text/html"
	case "application/json":
		return "application/json"
	case "application/octet-stream":
		return "application/octet-stream"
	}
	return "other-or-missing"
}
func debugf(_ Config, format string, args ...any) {
	log.F("[subscription-debug] "+format, args...)
}
