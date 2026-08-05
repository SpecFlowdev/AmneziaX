package xray

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestWireGuardKeysAreValidAndDerive(t *testing.T) {
	priv, pub, err := NewWireGuardKey()
	if err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]string{"private": priv, "public": pub} {
		raw, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			t.Fatalf("%s key is not base64: %v", name, err)
		}
		if len(raw) != 32 {
			t.Fatalf("%s key is %d bytes, want 32", name, len(raw))
		}
	}

	// The whole point: the stored private key must derive exactly the public
	// key the peer was told about. If these ever disagree the tunnel silently
	// never comes up.
	derived, err := WireGuardPublicKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	if derived != pub {
		t.Fatalf("derived %s, want %s", derived, pub)
	}
}

func TestWireGuardKeysDiffer(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		priv, _, err := NewWireGuardKey()
		if err != nil {
			t.Fatal(err)
		}
		if seen[priv] {
			t.Fatal("a private key repeated")
		}
		seen[priv] = true
	}
}

func TestWireGuardPublicKeyRejectsRubbish(t *testing.T) {
	for _, in := range []string{"", "not base64!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := WireGuardPublicKey(in); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
}

func TestWireGuardAddressesAreDistinctAndSkipTheServer(t *testing.T) {
	seen := map[string]bool{}
	for i := int64(0); i < 5000; i++ {
		addr := WireGuardAddress(i)
		if seen[addr] {
			t.Fatalf("index %d reused %s", i, addr)
		}
		seen[addr] = true

		if !strings.HasPrefix(addr, "10.66.") || !strings.HasSuffix(addr, "/32") {
			t.Fatalf("index %d gave %s", i, addr)
		}
		// .0 and .1 of the whole range belong to the network and the server.
		if strings.HasPrefix(addr, "10.66.0.0") || strings.HasPrefix(addr, "10.66.0.1/") {
			t.Fatalf("index %d handed out a reserved address: %s", i, addr)
		}
	}
}
