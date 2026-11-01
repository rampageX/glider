package trojan

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"strings"
)

// Certificate pins are enforced even when CA verification was explicitly
// disabled by the provider. Never discard a pin while importing a URI.
func certificatePin(value string, names ...string) (func([][]byte, [][]*x509.Certificate) error, error) {
	if value == "" {
		return nil, nil
	}
	pin, err := hex.DecodeString(strings.ReplaceAll(value, ":", ""))
	if err != nil || len(pin) != sha256.Size {
		return nil, errors.New("invalid certificate SHA256 pin")
	}
	return func(raw [][]byte, _ [][]*x509.Certificate) error {
		for i, cert := range raw {
			sum := sha256.Sum256(cert)
			if subtle.ConstantTimeCompare(sum[:], pin) == 1 {
				if i == 0 {
					return nil
				}
				ca, err := x509.ParseCertificate(cert)
				if err != nil || !ca.IsCA {
					continue
				}
				leaf, err := x509.ParseCertificate(raw[0])
				if err != nil {
					return errors.New("invalid pinned certificate chain")
				}
				roots, intermediates := x509.NewCertPool(), x509.NewCertPool()
				roots.AddCert(ca)
				for _, data := range raw[1:] {
					if parsed, e := x509.ParseCertificate(data); e == nil {
						intermediates.AddCert(parsed)
					}
				}
				name := ""
				if len(names) > 0 {
					name = names[0]
				}
				if name == "" {
					return errors.New("CA pin requires serverName")
				}
				_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: name})
				return err
			}
		}
		return errors.New("certificate SHA256 pin mismatch")
	}, nil
}
