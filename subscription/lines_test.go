package subscription

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/nadoo/glider/proxy/reject"
	"github.com/nadoo/glider/rule"
)

func TestMixedSSRSubscriptionHeaders(t *testing.T) {
	var lines []string
	lines = append(lines, "MAX=0", "STATUS=provider notice with private data")
	for i := 0; i < 117; i++ {
		lines = append(lines, "ssr://"+b64("example.test:443:origin:aes-128-cfb:plain:"+b64("password")+"/?remarks="+b64(fmt.Sprintf("美国 %d", i))))
	}
	plain := strings.Join(lines, "\r\n")
	for _, body := range []string{plain, base64.StdEncoding.EncodeToString([]byte(plain))} {
		nodes, skip, e := Parse([]byte(body))
		if e != nil || len(nodes) != 117 || skip != 0 {
			t.Fatalf("nodes=%d skip=%d err=%v", len(nodes), skip, e)
		}
	}
}
func TestMixedSSUnsupportedAndComments(t *testing.T) {
	lines := []string{"\ufeff# provider header", ";comment", "// comment", "REMARKS=notice"}
	for i := 0; i < 28; i++ {
		lines = append(lines, ss(fmt.Sprintf("美国 %d", i)))
	}
	for i := 0; i < 12; i++ {
		lines = append(lines, "vmess://"+b64("synthetic-unsupported"))
	}
	plain := strings.Join(lines, "\r\n")
	for _, body := range []string{plain, b64(plain)} {
		nodes, skip, e := Parse([]byte(body))
		if e != nil || len(nodes) != 28 || skip != 12 {
			t.Fatalf("nodes=%d skip=%d err=%v", len(nodes), skip, e)
		}
	}
}
func TestUnknownAndMalformedRemainAtomic(t *testing.T) {
	for _, line := range []string{"arbitrary provider text", "unknown-protocol://SECRET", "MAX=not-a-number", "ssr://invalid", "ss://invalid"} {
		nodes, _, e := Parse([]byte(b64("MAX=0\n" + ss("美国") + "\n" + line)))
		if e == nil || nodes != nil {
			t.Fatalf("accepted partial input for %s", lineClass(line))
		}
		if strings.Contains(e.Error(), "SECRET") {
			t.Fatal("unknown text leaked")
		}
	}
	if n, _, e := Parse([]byte("MAX=0\nSTATUS=ok\nvmess://unsupported")); e == nil || n != nil {
		t.Fatal("non-SS/SSR input accepted")
	}
}
func TestMixedColdStartWithoutSnapshot(t *testing.T) {
	text := b64("MAX=0\nSTATUS=notice\n" + ss("美国") + "\nhysteria2://unsupported")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, text) }))
	defer server.Close()
	c := testConfig(t.TempDir(), server.URL)
	c.Matches = []string{"美国"}
	if _, e := os.Stat(c.Output); !os.IsNotExist(e) {
		t.Fatal("snapshot unexpectedly exists")
	}
	p := rule.NewProxy([]string{"reject://#name=subscription-pending"}, &rule.Strategy{Strategy: "lha", Check: "disable", DialTimeout: 1}, nil)
	defer p.Close()
	m, e := New(c, p.ReplaceMain)
	if e != nil {
		t.Fatal(e)
	}
	m.Refresh(context.Background())
	if p.StatusSnapshot()[0].Forwarders[0].Name != "美国" {
		t.Fatal("cold start did not activate node")
	}
	defs, e := ReadSnapshot(c.Output)
	if e != nil || len(defs) != 1 {
		t.Fatal("no generated snapshot", e)
	}
}
func TestLineClassificationNeverPrintsPrivateContent(t *testing.T) {
	const secret = "SECRET_TOKEN_VALUE"
	samples := map[string]string{
		"#" + secret:            "comment",
		"STATUS=" + secret:      "metadata-status",
		"MAX=" + secret:         "invalid-metadata-max",
		"vmess://" + secret:     "unsupported-vmess",
		secret + "://" + secret: "unknown-scheme",
		secret:                  "unknown-text",
	}
	for input, want := range samples {
		if got := lineClass(input); got != want {
			t.Fatal(got, want)
		}
	}
	summary := bodySummary([]byte(b64("MAX=0\nSTATUS=" + secret + "\n" + ss(secret) + "\nvmess://" + secret)))
	for _, want := range []string{"metadata-max:1", "metadata-status:1", "ss:1", "unsupported-vmess:1"} {
		if !strings.Contains(summary, want) {
			t.Fatal(summary)
		}
	}
	if strings.Contains(summary, secret) {
		t.Fatal("summary leaked private content")
	}
}
