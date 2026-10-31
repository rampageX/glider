package subscription

import (
	"encoding/base64"
	_ "github.com/nadoo/glider/proxy/socks5"
	_ "github.com/nadoo/glider/proxy/ss"
	"github.com/nadoo/glider/rule"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
func ss(name string) string {
	return "ss://" + b64("aes-128-gcm:password") + "@example.test:443/#" + url.PathEscape(name)
}
func TestSSFormats(t *testing.T) {
	pass := "密码 空格@:#&+,百分%25"
	name := "  '香港 +, @:#&%25'  "
	user := "aes-128-gcm:" + pass
	for _, s := range []string{
		"ss://" + base64.StdEncoding.EncodeToString([]byte(user)) + "@example.test:443/#" + url.PathEscape(name),
		"ss://" + b64(user) + "@example.test:443#" + url.PathEscape(name),
		"ss://" + b64(user+"@example.test:443") + "#" + url.PathEscape(name),
		"ss://" + url.PathEscape(user) + "@example.test:443#" + url.PathEscape(name),
	} {
		nodes, _, e := Parse([]byte(s))
		if e != nil {
			t.Fatal(e)
		}
		f := nodes[0].Forward
		if strings.Contains(strings.SplitN(f, "#", 2)[0], ",") {
			t.Fatal("chain separator")
		}
		u, e := url.Parse(strings.SplitN(f, "#", 2)[0])
		if e != nil {
			t.Fatal(e)
		}
		p, _ := u.User.Password()
		if p != pass {
			t.Fatalf("password lost %q", p)
		}
		fw, e := rule.ForwarderFromURL(f, "", time.Second, time.Second)
		if e != nil || fw.Name() != name {
			t.Fatalf("name lost %q %v", fw.Name(), e)
		}
	}
	for _, s := range []string{"ss://aes-128-gcm:plain+password@example.test:443#日本", "ss://" + b64("none:p@[::1]:80")} {
		if _, _, e := Parse([]byte(s)); e != nil {
			t.Fatal(e)
		}
	}
}
func TestSSRIndependent(t *testing.T) {
	makeLink := func(pass, name, group, proto, obfs string) string {
		return "ssr://" + b64("example.test:443:auth_sha1_v4:aes-128-cfb:http_simple:"+b64(pass)+"/?remarks="+b64(name)+"&group="+b64(group)+"&protoparam="+b64(proto)+"&obfsparam="+b64(obfs))
	}
	a := makeLink("密@:#&+, %25", "香港 +, %", "组一", "1:密码+,&%", "a,b +%")
	b := makeLink("two", "日本", "组二", "", "")
	n, _, e := Parse([]byte(a + "\r\n" + b))
	if e != nil {
		t.Fatal(e)
	}
	for i, want := range []string{"1:密码+,&%", ""} {
		u, _ := url.Parse(strings.SplitN(n[i].Forward, "#", 2)[0])
		if u.Query().Get("protocol_param") != want {
			t.Fatal("parameter bleed")
		}
	}
	if got := Filter(n, []string{"组一"}, nil); len(got) != 1 {
		t.Fatal(got)
	}
}
func TestEnvelopeAtomicAndUnsupported(t *testing.T) {
	text := ss("香港") + "\r\n" + ss("日本")
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		n, _, e := Parse([]byte(enc.EncodeToString([]byte(text))))
		if e != nil || len(n) != 2 {
			t.Fatal(e)
		}
	}
	if n, _, e := Parse([]byte(text + "\nssr://bad")); e == nil || n != nil {
		t.Fatal("accepted partial input")
	}
	unsupported := "ss://" + b64("bad-cipher:p") + "@x:1"
	n, skip, e := Parse([]byte(unsupported + "\n" + ss("香港") + "\nss://" + b64("aes-128-gcm:p") + "@x:1/?plugin=obfs-local"))
	if e != nil || skip != 2 || len(n) != 1 {
		t.Fatalf("skip=%d %v", skip, e)
	}
}
func TestFilterOrderDuplicates(t *testing.T) {
	n, _, e := Parse([]byte(ss("香港") + "\n" + ss("香港遊戲") + "\n" + ss("日本") + "\n" + ss("香港")))
	if e != nil {
		t.Fatal(e)
	}
	out := Filter(n, []string{"香港", "日本"}, []string{"遊戲"})
	if len(out) != 3 || out[0] != out[2] || out[1] != n[2].Forward {
		t.Fatal(out)
	}
}

func TestGeneratedDocumentExamples(t *testing.T) {
	data, e := os.ReadFile("../docs/native-subscriptions.md")
	if e != nil {
		t.Fatal(e)
	}
	var defs []string
	for _, line := range strings.Split(string(data), "\n") {
		if s, ok := strings.CutPrefix(line, "forward="); ok {
			defs = append(defs, s)
		}
	}
	if len(defs) != 2 || Validate(defs) != nil {
		t.Fatal("document forward examples invalid")
	}
	path := filepath.Join(t.TempDir(), "nodes.conf")
	if e := atomicWrite(path, Render(defs)); e != nil {
		t.Fatal(e)
	}
	restored, e := ReadSnapshot(path)
	if e != nil || !reflect.DeepEqual(defs, restored) {
		t.Fatal("snapshot roundtrip", e)
	}
}
