package subscription

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	_ "github.com/nadoo/glider/proxy/anytls"
	_ "github.com/nadoo/glider/proxy/trojan"
)

func TestModernMapping(t *testing.T) {
	cases := []struct{ raw, scheme, name, sni, skip string }{
		{"anytls://p%40ss%3Aword@example.test:443?sni=sni.test&insecure=1#香港%20A%2CB", "anytls", "香港 A,B", "sni.test", "true"},
		{"trojan://pass@example.test:443?peer=sni.test&allowInsecure=0&type=tcp#日本", "trojan", "日本", "sni.test", ""},
		{"trojan://pass@[::1]:443?sni=sni.test&allowInsecure=1#IPv6", "trojan", "IPv6", "sni.test", "true"},
		{"vless://707f20ea-d4b8-4d1d-8e2e-2c86cb2ed97a@example.test:443?security=tls&sni=sni.test&type=tcp&encryption=none#VLESS", "vless", "VLESS", "sni.test", ""},
	}
	for _, c := range cases {
		for _, body := range []string{c.raw, b64(c.raw)} {
			ns, skipped, err := Parse([]byte(body))
			if err != nil || len(ns) != 1 || skipped != 0 {
				t.Fatalf("%s: count=%d skipped=%d err=%v", c.scheme, len(ns), skipped, err)
			}
			n := ns[0]
			u, err := url.Parse(strings.SplitN(n.Forward, "#", 2)[0])
			if err != nil || u.Scheme != c.scheme || n.Name != c.name || u.Query().Get("serverName") != c.sni || u.Query().Get("skipVerify") != c.skip {
				t.Fatalf("wrong %s mapping", c.scheme)
			}
			if strings.Contains(strings.SplitN(n.Forward, "#", 2)[0], ",") {
				t.Fatal("chain separator injected")
			}
			if err := Validate([]string{n.Forward}); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func TestV2rayNTCPDefaultsAndSingleLineEnvelope(t *testing.T) {
	// v2rayN includes these unused TCP fields in some REALITY/Vision exports.
	// The TLS fixture exercises the same normalization without requiring utls.
	link := "vless://707f20ea-d4b8-4d1d-8e2e-2c86cb2ed97a@example.test:443?type=tcp&encryption=none&security=tls&flow=xtls-rprx-vision&headerType=none&host=&path=#Example"
	lines := make([]string, 0, 44)
	for i := 0; i < 34; i++ {
		lines = append(lines, link)
	}
	for i := 0; i < 10; i++ {
		lines = append(lines, "hysteria2://unsupported@example.test:443")
	}
	text := base64.RawStdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))
	nodes, skipped, err := Parse([]byte(text))
	if err != nil || len(nodes) != 34 || skipped != 10 {
		t.Fatalf("v2rayN envelope nodes=%d skipped=%d err=%v", len(nodes), skipped, err)
	}
	if err := Validate([]string{nodes[0].Forward}); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(strings.SplitN(nodes[0].Forward, "#", 2)[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"headerType", "host", "path"} {
		if _, exists := u.Query()[key]; exists {
			t.Fatalf("inert %s leaked into canonical forwarder", key)
		}
	}
	for _, change := range [][2]string{{"headerType=none", "headerType=http"}, {"host=", "host=example.test"}, {"path=", "path=%2Ftest"}, {"type=tcp", "type=ws"}} {
		bad := strings.Replace(link, change[0], change[1], 1)
		if n, s, e := Parse([]byte(bad)); e == nil || n != nil || s != 1 {
			t.Fatalf("unsafe transport extension %s accepted: nodes=%d skipped=%d err=%v", change[1], len(n), s, e)
		}
	}
}
func TestModernUnsupportedAndMalformed(t *testing.T) {
	for _, raw := range []string{
		"trojan://p@example.test:443?type=ws", "anytls://p@example.test:443?plugin=unknown",
		"vless://id@example.test:443?security=tls&type=grpc", "vless://id@example.test:443?security=none&flow=xtls-rprx-vision",
		"trojan://p@example.test:443?alpn=h2",
	} {
		ns, skip, err := Parse([]byte(ss("ok") + "\n" + raw))
		if err != nil || len(ns) != 1 || skip != 1 {
			t.Fatalf("unsupported case: nodes=%d skip=%d err=%v", len(ns), skip, err)
		}
	}
	for _, raw := range []string{
		"trojan://p@example.test:443?insecure=maybe", "anytls://p@example.test:443?sni=a&peer=b",
		"trojan://p@example.test:443?insecure=1&skipVerify=false", "anytls://p@example.test:443?sni=a&sni=b",
		"trojan://p@example.test:0", "trojan://p@example.test:443?pcs=invalid", "vless://unsupported", "anytls://p@example.test:443?sni=%zz",
	} {
		ns, _, err := Parse([]byte(ss("ok") + "\n" + raw))
		if err == nil || ns != nil {
			t.Fatal("malformed input accepted")
		}
		if strings.Contains(err.Error(), "example.test") {
			t.Fatal("private URL leaked")
		}
	}
}
