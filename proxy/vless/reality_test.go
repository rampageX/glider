//go:build with_utls

package vless

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nadoo/glider/proxy"
)

// Use an independent implementation for REALITY + Vision interoperability.
// The executable is optional, never downloaded implicitly by a unit test.
func TestRealityVisionXrayInterop(t *testing.T) {
	binary := os.Getenv("GLIDER_XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("independent Xray executable not configured")
	}
	payload := strings.Repeat("Vision interoperability payload. ", 40000)
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, payload) }))
	target.Config.ErrorLog = log.New(io.Discard, "", 0)
	target.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	target.StartTLS()
	defer target.Close()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyString := base64.RawURLEncoding.EncodeToString(key.Bytes())
	pub := base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	const uuid = "707f20ea-d4b8-4d1d-8e2e-2c86cb2ed97a"
	config := map[string]any{
		"log": map[string]any{"loglevel": "none"},
		"inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": port, "protocol": "vless",
			"settings": map[string]any{"clients": []any{map[string]any{"id": uuid, "flow": "xtls-rprx-vision"}}, "decryption": "none"},
			"streamSettings": map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{
				"dest": target.Listener.Addr().String(), "serverNames": []string{"example.com"}, "privateKey": keyString, "shortIds": []string{"abcd"},
			}},
		}}, "outbounds": []any{map[string]any{"protocol": "freedom"}},
	}
	path := filepath.Join(t.TempDir(), "xray.json")
	b, _ := json.Marshal(config)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "run", "-c", path)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	ready := false
	for i := 0; i < 100; i++ {
		c, e := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if e == nil {
			c.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("Xray test server did not start")
	}
	direct, _ := proxy.NewDirect("", 3*time.Second, 15*time.Second)
	for _, fingerprint := range []string{"chrome", "firefox", "safari", "edge", "ios"} {
		t.Run(fingerprint, func(t *testing.T) {
			q := url.Values{"security": {"reality"}, "flow": {"xtls-rprx-vision"}, "serverName": {"example.com"}, "fp": {fingerprint}, "pbk": {pub}, "sid": {"abcd"}}
			d, err := NewVLessDialer("vless://"+uuid+"@"+addr+"?"+q.Encode(), direct)
			if err != nil {
				t.Fatal(err)
			}
			tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) { return d.Dial(network, addr) }}
			defer tr.CloseIdleConnections()
			client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
			response, err := client.Get(target.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			got, err := io.ReadAll(response.Body)
			if err != nil || string(got) != payload {
				t.Fatalf("large TLS payload failed: bytes=%d err=%v", len(got), err)
			}
			if fingerprint == "chrome" {
				udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
				if err != nil {
					t.Fatal(err)
				}
				defer udp.Close()
				go func() {
					buf := make([]byte, 4096)
					n, a, e := udp.ReadFromUDP(buf)
					if e == nil {
						udp.WriteToUDP(buf[:n], a)
					}
				}()
				packet, err := d.DialUDP("udp", udp.LocalAddr().String())
				if err != nil {
					t.Fatal(err)
				}
				defer packet.Close()
				packet.SetDeadline(time.Now().Add(3 * time.Second))
				if _, err := packet.WriteTo([]byte("XUDP-echo"), udp.LocalAddr()); err != nil {
					t.Fatal(err)
				}
				buf := make([]byte, 64)
				n, _, err := packet.ReadFrom(buf)
				if err != nil || string(buf[:n]) != "XUDP-echo" {
					t.Fatalf("Vision XUDP failed: %v", err)
				}
			}
			// A valid TLS destination certificate must never replace REALITY auth.
			q.Set("sid", "ffff")
			bad, err := NewVLessDialer("vless://"+uuid+"@"+addr+"?"+q.Encode(), direct)
			if err != nil {
				t.Fatal(err)
			}
			if c, err := bad.Dial("tcp", target.Listener.Addr().String()); err == nil {
				c.Close()
				t.Fatal("wrong short ID authenticated")
			}
		})
	}
}
