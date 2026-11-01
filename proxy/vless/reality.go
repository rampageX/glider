//go:build with_utls

package vless

// REALITY ClientHello authentication follows XTLS/REALITY and sing-box's
// common/tls/reality_client.go (GPL-3.0). Vision framing is provided by
// github.com/sagernet/sing-vmess. Keep the uTLS dependency pinned: handshake
// internals and Vision's TLS record transition must be updated together.
import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	utls "github.com/metacubex/utls"
	"golang.org/x/crypto/hkdf"
)

const realityAvailable = true

func realityHandshake(conn net.Conn, q url.Values) (net.Conn, error) {
	id := utls.HelloChrome_Auto
	switch q.Get("fp") {
	case "firefox":
		id = utls.HelloFirefox_Auto
	case "safari":
		id = utls.HelloSafari_Auto
	case "edge":
		id = utls.HelloEdge_Auto
	case "ios":
		id = utls.HelloIOS_Auto
	}
	var authKey []byte
	verified := false
	config := &utls.Config{
		ServerName: q.Get("serverName"), MinVersion: utls.VersionTLS13,
		InsecureSkipVerify: true, SessionTicketsDisabled: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("REALITY missing certificate")
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return errors.New("REALITY invalid certificate")
			}
			if pub, ok := cert.PublicKey.(ed25519.PublicKey); ok {
				h := hmac.New(sha512.New, authKey)
				h.Write(pub)
				if hmac.Equal(h.Sum(nil), cert.Signature) {
					verified = true
					return nil
				}
			}
			return errors.New("REALITY authentication failed")
		},
	}
	c := utls.UClient(conn, config, id)
	if err := c.BuildHandshakeState(); err != nil {
		return nil, err
	}
	// Use ordinary X25519, whose private key the authentication scheme binds to.
	for _, ext := range c.Extensions {
		switch ext := ext.(type) {
		case *utls.SupportedCurvesExtension:
			filtered := ext.Curves[:0]
			for _, curve := range ext.Curves {
				if curve != utls.X25519MLKEM768 {
					filtered = append(filtered, curve)
				}
			}
			ext.Curves = filtered
		case *utls.KeyShareExtension:
			filtered := ext.KeyShares[:0]
			for _, share := range ext.KeyShares {
				if share.Group != utls.X25519MLKEM768 {
					filtered = append(filtered, share)
				}
			}
			ext.KeyShares = filtered
		}
	}
	if err := c.BuildHandshakeState(); err != nil {
		return nil, err
	}
	if alpn := q.Get("alpn"); alpn != "" {
		for _, ext := range c.Extensions {
			if a, ok := ext.(*utls.ALPNExtension); ok {
				a.AlpnProtocols = strings.Split(alpn, ",")
			}
		}
		if err := c.BuildHandshakeState(); err != nil {
			return nil, err
		}
	}
	hello := c.HandshakeState.Hello
	if hello == nil || len(hello.Raw) < 71 || len(hello.Random) != 32 {
		return nil, errors.New("unsupported REALITY ClientHello")
	}
	hello.SessionId = make([]byte, 32)
	copy(hello.Raw[39:71], hello.SessionId)
	hello.SessionId[0], hello.SessionId[1], hello.SessionId[2] = 1, 8, 1
	binary.BigEndian.PutUint32(hello.SessionId[4:8], uint32(time.Now().Unix()))
	sid, err := hex.DecodeString(q.Get("sid"))
	if err != nil || len(sid) > 8 {
		return nil, errors.New("invalid REALITY short ID")
	}
	copy(hello.SessionId[8:16], sid)
	keyBytes, err := base64.RawURLEncoding.DecodeString(q.Get("pbk"))
	if err != nil {
		return nil, errors.New("invalid REALITY public key")
	}
	publicKey, err := ecdh.X25519().NewPublicKey(keyBytes)
	if err != nil {
		return nil, errors.New("invalid REALITY public key")
	}
	keys := c.HandshakeState.State13.KeyShareKeys
	if keys == nil || keys.Ecdhe == nil {
		return nil, errors.New("REALITY requires X25519 key share")
	}
	authKey, err = keys.Ecdhe.ECDH(publicKey)
	if err != nil {
		return nil, errors.New("invalid REALITY key exchange")
	}
	if _, err = io.ReadFull(hkdf.New(sha256.New, authKey, hello.Random[:20], []byte("REALITY")), authKey); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(authKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	aead.Seal(hello.SessionId[:0], hello.Random[20:], bytes.Clone(hello.SessionId[:16]), hello.Raw)
	copy(hello.Raw[39:71], hello.SessionId)
	if err = c.Handshake(); err != nil {
		return nil, err
	}
	if !verified {
		return nil, errors.New("REALITY authentication failed")
	}
	return c, nil
}
