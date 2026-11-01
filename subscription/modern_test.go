package subscription

import (
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
