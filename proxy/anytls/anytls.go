package anytls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/nadoo/glider/proxy"
)

type AnyTLS struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy

	addr       string
	password   string
	withTLS    bool
	serverName string
	skipVerify bool
	certFile   string
	keyFile    string
	fallback   string
	tlsConfig  *tls.Config

	synackTimeout time.Duration
	padding       paddingScheme
}

func NewAnyTLS(s string, d proxy.Dialer, p proxy.Proxy) (*AnyTLS, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("[anytls] parse url err: %s", err)
	}

	query := u.Query()
	password := ""
	if u.User != nil {
		password = u.User.Username()
	}

	serverName := query.Get("serverName")
	if serverName == "" {
		serverName = query.Get("sni")
	}

	skipVerify := query.Get("skipVerify") == "true"
	if insecure := query.Get("insecure"); insecure == "1" || insecure == "true" {
		skipVerify = true
	}

	a := &AnyTLS{
		dialer:        d,
		proxy:         p,
		addr:          u.Host,
		password:      password,
		withTLS:       true,
		serverName:    serverName,
		skipVerify:    skipVerify,
		certFile:      query.Get("cert"),
		keyFile:       query.Get("key"),
		fallback:      query.Get("fallback"),
		synackTimeout: 10 * time.Second,
	}

	if a.password == "" {
		return nil, errors.New("[anytls] password must be specified")
	}

	if a.addr != "" {
		host, port, splitErr := net.SplitHostPort(a.addr)
		if splitErr != nil || port == "" {
			host = u.Hostname()
			a.addr = net.JoinHostPort(host, "443")
		}
		if a.serverName == "" {
			a.serverName = host
		}
	}

	if timeout := query.Get("synackTimeout"); timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, fmt.Errorf("[anytls] invalid synackTimeout: %s", err)
		}
		a.synackTimeout = d
	}

	if scheme := query.Get("paddingScheme"); scheme != "" {
		a.padding, err = parsePaddingScheme(scheme)
	} else {
		a.padding, err = parsePaddingScheme(defaultPaddingScheme)
	}
	if err != nil {
		return nil, fmt.Errorf("[anytls] invalid padding scheme: %s", err)
	}

	return a, nil
}

func (s *AnyTLS) Addr() string {
	if s.addr == "" && s.dialer != nil {
		return s.dialer.Addr()
	}
	return s.addr
}

func loadClientTLSConfig(serverName, certFile string, skipVerify bool) (*tls.Config, error) {
	conf := &tls.Config{ServerName: serverName, InsecureSkipVerify: skipVerify, MinVersion: tls.VersionTLS12}
	if certFile != "" {
		certData, err := os.ReadFile(certFile)
		if err != nil {
			return nil, fmt.Errorf("[anytls] read cert file error: %s", err)
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(certData) {
			return nil, fmt.Errorf("[anytls] can not append cert file: %s", certFile)
		}
		conf.RootCAs = certPool
	}
	return conf, nil
}

func init() {
	proxy.AddUsage("anytls", `
AnyTLS client scheme:
  anytls://password@host:port[?serverName=SERVERNAME][&skipVerify=true][&cert=PATH][&synackTimeout=10s]
  anytls://password@host:port[?sni=SERVERNAME][&insecure=1]   (legacy aliases kept for compatibility)
  anytlsc://password@host:port     (cleartext, without TLS)

AnyTLS server scheme:
  anytls://password@host:port?cert=PATH&key=PATH[&fallback=127.0.0.1:80]
  anytlsc://password@host:port[?fallback=127.0.0.1:80]     (cleartext, without TLS)
`)
}
