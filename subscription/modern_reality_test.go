//go:build with_utls

package subscription

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"testing"
)

func TestV2rayNRealityVisionTCPDefaults(t *testing.T) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	q := url.Values{
		"type":       {"tcp"},
		"encryption": {"none"},
		"security":   {"reality"},
		"flow":       {"xtls-rprx-vision"},
		"pbk":        {base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())},
		"sid":        {"abcd"},
		"sni":        {"example.test"},
		"fp":         {"chrome"},
		"headerType": {"none"},
		"host":       {""},
		"path":       {""},
	}
	link := "vless://707f20ea-d4b8-4d1d-8e2e-2c86cb2ed97a@example.test:443?" + q.Encode() + "#Example"
	nodes, skipped, err := Parse([]byte(base64.StdEncoding.EncodeToString([]byte(link))))
	if err != nil || len(nodes) != 1 || skipped != 0 {
		t.Fatalf("REALITY + Vision nodes=%d skipped=%d err=%v", len(nodes), skipped, err)
	}
	if err := Validate([]string{nodes[0].Forward}); err != nil {
		t.Fatal(err)
	}
}
