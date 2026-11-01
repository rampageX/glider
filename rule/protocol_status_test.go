package rule

import "testing"

func TestForwarderProtocolFromSchemes(t *testing.T) {
	for _, c := range []struct{ url, want string }{
		{"anytls://p@example.test:443#name=VLESS", "ANYTLS"},
		{"trojan://p@example.test:443#name=SS", "TROJAN"},
		{"vless://id@example.test:443?security=reality&flow=xtls-rprx-vision#name=HK", "VLESS"},
		{"tls://example.test:443,ws://example.test/path,vless://id@#name=HK", "TLS → WS → VLESS"},
		{"ss://aes-128-gcm:p@example.test:443#name=anytls", "SS"},
		{"private-token://secret@example.test:443", "UNKNOWN"},
	} {
		f := &Forwarder{url: c.url}
		if got := f.Protocol(); got != c.want {
			t.Fatalf("got %s want %s", got, c.want)
		}
	}
}
