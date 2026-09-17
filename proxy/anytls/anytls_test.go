package anytls

import "testing"

func TestNewAnyTLSClientURLCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		addr       string
		serverName string
		skipVerify bool
	}{
		{
			name:       "new options",
			url:        "anytls://secret@example.com:8443?serverName=cdn.example.com&skipVerify=true",
			addr:       "example.com:8443",
			serverName: "cdn.example.com",
			skipVerify: true,
		},
		{
			name:       "legacy aliases",
			url:        "anytls://secret@example.com?sni=legacy.example.com&insecure=1",
			addr:       "example.com:443",
			serverName: "legacy.example.com",
			skipVerify: true,
		},
		{
			name:       "defaults",
			url:        "anytls://secret@example.com",
			addr:       "example.com:443",
			serverName: "example.com",
			skipVerify: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewAnyTLS(tt.url, nil, nil)
			if err != nil {
				t.Fatalf("NewAnyTLS() error = %v", err)
			}
			if a.addr != tt.addr {
				t.Fatalf("addr = %q, want %q", a.addr, tt.addr)
			}
			if a.serverName != tt.serverName {
				t.Fatalf("serverName = %q, want %q", a.serverName, tt.serverName)
			}
			if a.skipVerify != tt.skipVerify {
				t.Fatalf("skipVerify = %v, want %v", a.skipVerify, tt.skipVerify)
			}
		})
	}
}

func TestNewAnyTLSRequiresPassword(t *testing.T) {
	if _, err := NewAnyTLS("anytls://example.com:443", nil, nil); err == nil {
		t.Fatal("expected password validation error")
	}
}
