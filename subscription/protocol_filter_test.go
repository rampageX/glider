package subscription

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProtocolAllowlistAndSnapshot(t *testing.T) {
	c := Config{Protocols: []string{" AnyTLS, TROJAN ", "anytls"}, Matches: []string{"香港"}}
	if err := c.normalizeProtocols(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Protocols, []string{"anytls", "trojan"}) {
		t.Fatal("normalization failed")
	}
	nodes := []Node{
		{Forward: "ss://aes-128-gcm:p@example.test:443#name=anytls", Name: "香港 anytls"},
		{Forward: "anytls://p@example.test:443#name=HK", Name: "香港 A"},
		{Forward: "trojan://p@example.test:443#name=HK", Name: "香港 B"},
		{Forward: "anytls://p@example.test:443#name=JP", Name: "日本"},
	}
	if got := c.Filter(nodes); !reflect.DeepEqual(got, []string{nodes[1].Forward, nodes[2].Forward}) {
		t.Fatal("name or protocol filtering failed")
	}
	c.Output = filepath.Join(t.TempDir(), "snapshot.conf")
	if err := os.WriteFile(c.Output, Render([]string{nodes[0].Forward, nodes[1].Forward}), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadSnapshot()
	if err != nil || !reflect.DeepEqual(got, []string{nodes[1].Forward}) {
		t.Fatal("old mixed snapshot was not filtered")
	}
	c.Protocols = []string{"vless"}
	if _, err := c.ReadSnapshot(); err == nil {
		t.Fatal("excluded snapshot accepted")
	}
	c.Protocols = nil
	if got, err := c.ReadSnapshot(); err != nil || len(got) != 2 {
		t.Fatal("default behavior changed")
	}
	if _, err := New(Config{Protocols: []string{"anytlss"}}, nil); err == nil {
		t.Fatal("typo accepted")
	}
}
func TestProtocolFilterManagerStatusAndRetain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, ss("香港")+"\nanytls://password@example.test:443?sni=example.test#香港")
	}))
	defer server.Close()
	c := testConfig(t.TempDir(), server.URL)
	c.Protocols = []string{"anytls"}
	var applied []string
	m, err := New(c, func(defs []string, persist func() error) (bool, error) {
		applied = append([]string(nil), defs...)
		return true, persist()
	})
	if err != nil {
		t.Fatal(err)
	}
	m.Refresh(context.Background())
	if len(applied) != 1 || !strings.HasPrefix(applied[0], "anytls://") {
		t.Fatal("wrong protocol activated")
	}
	if s := m.Snapshot(); s.Inputs[0].MatchedNodes != 1 {
		t.Fatal("status did not apply protocol filter")
	}
	before, _ := os.ReadFile(c.Output)
	m.conf.Protocols = []string{"vless"}
	m.Refresh(context.Background())
	after, _ := os.ReadFile(c.Output)
	if string(before) != string(after) || len(applied) != 1 {
		t.Fatal("empty selection overwrote valid state")
	}
}
