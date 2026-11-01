package vless

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/nadoo/glider/proxy"
	sv "github.com/nadoo/glider/proxy/vless/internal/vision"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
)

// ValidateClientURL prevents unsupported transport/security extensions from
// being silently downgraded to the historical plaintext VLESS implementation.
func ValidateClientURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil || u.User.Username() == "" {
		return errors.New("invalid VLESS URL")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return errors.New("invalid VLESS query")
	}
	for k, vs := range q {
		switch k {
		case "security", "flow", "serverName", "skipVerify", "alpn", "fp", "pbk", "sid", "spx", "fallback":
		default:
			return errors.New("unsupported VLESS option")
		}
		if len(vs) != 1 {
			return errors.New("duplicate VLESS option")
		}
	}
	security, flow := q.Get("security"), q.Get("flow")
	if (security == "tls" || security == "reality") && q.Get("fallback") != "" {
		return errors.New("fallback is a server-only option")
	}
	if security != "" && security != "none" && security != "tls" && security != "reality" {
		return errors.New("unsupported VLESS security")
	}
	if flow != "" && flow != "xtls-rprx-vision" {
		return errors.New("unsupported VLESS flow")
	}
	if flow != "" && security != "tls" && security != "reality" {
		return errors.New("Vision requires TLS or REALITY")
	}
	if q.Get("skipVerify") != "" && q.Get("skipVerify") != "true" && q.Get("skipVerify") != "false" {
		return errors.New("invalid skipVerify")
	}
	if security == "reality" {
		if !realityAvailable {
			return errors.New("REALITY requires a with_utls build")
		}
		key, e := base64.RawURLEncoding.DecodeString(q.Get("pbk"))
		if e != nil || len(key) != 32 {
			return errors.New("invalid REALITY public key")
		}
		sid, e := hex.DecodeString(q.Get("sid"))
		if e != nil || len(sid) > 8 {
			return errors.New("invalid REALITY short ID")
		}
		if q.Get("serverName") == "" {
			return errors.New("REALITY requires serverName")
		}
		if q.Get("skipVerify") == "true" {
			return errors.New("REALITY cannot skip authentication")
		}
	} else if q.Get("pbk") != "" || q.Get("sid") != "" || q.Get("spx") != "" {
		return errors.New("REALITY parameters without REALITY")
	}
	if fp := q.Get("fp"); fp != "" {
		if security != "reality" {
			return errors.New("fingerprint currently requires REALITY")
		}
		switch fp {
		case "chrome", "firefox", "safari", "edge", "ios":
		default:
			return errors.New("unsupported fingerprint")
		}
	}
	_, err = StrToUUID(u.User.Username())
	if err != nil {
		return errors.New("invalid VLESS UUID")
	}
	return nil
}

type modernClient struct {
	dialer proxy.Dialer
	addr   string
	query  url.Values
	client *sv.Client
}

func newModernClient(raw string, d proxy.Dialer) (proxy.Dialer, error) {
	if err := ValidateClientURL(raw); err != nil {
		return nil, err
	}
	u, _ := url.Parse(raw)
	c, err := sv.NewClient(u.User.Username(), u.Query().Get("flow"), logger.NOP())
	if err != nil {
		return nil, errors.New("invalid VLESS client")
	}
	return &modernClient{d, u.Host, u.Query(), c}, nil
}
func (c *modernClient) Addr() string { return c.addr }
func (c *modernClient) connect() (net.Conn, error) {
	conn, err := c.dialer.Dial("tcp", c.addr)
	if err != nil {
		return nil, err
	}
	// Bound handshakes independently of the target connection's lifetime.
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	if c.query.Get("security") == "reality" {
		secured, e := realityHandshake(conn, c.query)
		if e != nil {
			conn.Close()
			return nil, e
		}
		secured.SetDeadline(time.Time{})
		return secured, nil
	}
	name := c.query.Get("serverName")
	if name == "" {
		name, _, _ = net.SplitHostPort(c.addr)
	}
	config := &tls.Config{ServerName: name, InsecureSkipVerify: c.query.Get("skipVerify") == "true", MinVersion: tls.VersionTLS12}
	if c.query.Get("flow") != "" {
		config.MinVersion = tls.VersionTLS13
	}
	if a := c.query.Get("alpn"); a != "" {
		config.NextProtos = strings.Split(a, ",")
	}
	secured := tls.Client(conn, config)
	if err := secured.Handshake(); err != nil {
		conn.Close()
		return nil, err
	}
	secured.SetDeadline(time.Time{})
	return secured, nil
}
func (c *modernClient) Dial(network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, proxy.ErrNotSupported
	}
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	result, err := c.client.DialConn(conn, M.ParseSocksaddr(addr))
	if err != nil {
		conn.Close()
	}
	return result, err
}
func (c *modernClient) DialUDP(network, addr string) (net.PacketConn, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	if c.query.Get("flow") != "" {
		result, e := c.client.DialEarlyXUDPPacketConn(conn, M.ParseSocksaddr(addr))
		if e != nil {
			conn.Close()
		}
		return result, e
	}
	result, e := c.client.DialEarlyPacketConn(conn, M.ParseSocksaddr(addr))
	if e != nil {
		conn.Close()
	}
	return result, e
}
