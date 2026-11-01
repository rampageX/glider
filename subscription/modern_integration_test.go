package subscription

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/rule"
)

func TestImportedTLSProtocolsTransfer(t *testing.T) {
	payload := strings.Repeat("subscription transport integration", 4000)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, payload) }))
	defer target.Close()
	certSource := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer certSource.Close()
	cert := certSource.TLS.Certificates[0]
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0600)
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600)
	for _, scheme := range []string{"anytls", "trojan"} {
		t.Run(scheme, func(t *testing.T) {
			p := rule.NewProxy([]string{"direct://"}, &rule.Strategy{Strategy: "lha", Check: "disable", DialTimeout: 3}, nil)
			defer p.Close()
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			q := url.Values{"cert": {certPath}, "key": {keyPath}}
			server, err := proxy.ServerFromURL(scheme+"://password@"+ln.Addr().String()+"?"+q.Encode(), p)
			if err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var conns []net.Conn
			defer func() {
				ln.Close()
				mu.Lock()
				defer mu.Unlock()
				for _, c := range conns {
					c.Close()
				}
			}()
			go func() {
				for {
					c, e := ln.Accept()
					if e != nil {
						return
					}
					mu.Lock()
					conns = append(conns, c)
					mu.Unlock()
					go server.Serve(c)
				}
			}()
			query := "sni=example.com&insecure=1"
			if scheme == "trojan" {
				sum := sha256.Sum256(cert.Certificate[0])
				query = "peer=example.com&allowInsecure=1&type=tcp&pcs=" + hex.EncodeToString(sum[:])
			}
			ns, _, err := Parse([]byte(scheme + "://password@" + ln.Addr().String() + "?" + query + "#测试节点"))
			if err != nil {
				t.Fatal(err)
			}
			f, err := rule.ForwarderFromURL(ns[0].Forward, "", 3*time.Second, 10*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			tr := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) { return f.Dial(network, addr) }}
			defer tr.CloseIdleConnections()
			response, err := (&http.Client{Transport: tr, Timeout: 10 * time.Second}).Get(target.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			got, err := io.ReadAll(response.Body)
			if err != nil || string(got) != payload {
				t.Fatal("imported TLS protocol payload mismatch")
			}
			if scheme == "trojan" {
				bad := strings.Replace(ns[0].Forward, "pcs=", "pcs=00", 1)
				if _, err := rule.ForwarderFromURL(bad, "", time.Second, time.Second); err == nil {
					t.Fatal("malformed pin accepted")
				}
			}
		})
	}
}
