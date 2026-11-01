package trojan

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestPinEnforced(t *testing.T) {
	cert := []byte("synthetic certificate")
	sum := sha256.Sum256(cert)
	check, err := certificatePin(hex.EncodeToString(sum[:]))
	if err != nil || check([][]byte{cert}, nil) != nil {
		t.Fatal("matching certificate rejected")
	}
	if check([][]byte{[]byte("wrong certificate")}, nil) == nil || check(nil, nil) == nil {
		t.Fatal("pin bypassed")
	}
	if _, err := certificatePin("invalid"); err == nil {
		t.Fatal("invalid pin accepted")
	}
}
