package xray

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// WireGuard keys are raw Curve25519, base64 in every config file and every
// implementation. Generating them here keeps the panel the only place a private
// key exists in readable form, exactly like the other per-user secrets.

// NewWireGuardKey returns a private key and the public key derived from it.
func NewWireGuardKey() (private, public string, err error) {
	var priv [32]byte
	if _, err = rand.Read(priv[:]); err != nil {
		return "", "", err
	}
	// Clamping is what every WireGuard implementation does to a fresh private
	// key. Skipping it produces a key that still works but is not the one the
	// peer derives its public key from.
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(priv[:]),
		base64.StdEncoding.EncodeToString(pub), nil
}

// WireGuardPublicKey derives the public half of a stored private key. The server
// key lives in the operator's profile document, and a client config has to name
// the server's *public* key — so it is computed rather than asked for, which is
// one fewer field to get wrong.
func WireGuardPublicKey(privateKey string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		return "", fmt.Errorf("wireguard key is not base64: %w", err)
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("wireguard key is %d bytes, want 32", len(raw))
	}
	pub, err := curve25519.X25519(raw, curve25519.Basepoint)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}

// WireGuardAddress maps a user's index onto an address inside the tunnel.
//
// 10.66.x.y gives roughly 65 000 subscribers per deployment. The first usable
// host is .2 because .0 and .1 are conventionally the network and the server,
// and handing a subscriber the address the server answers on is a fault that
// only shows up under load.
func WireGuardAddress(index int64) string {
	n := index%65024 + 512 // skip the first 512, keeping .0/.1 clear
	return fmt.Sprintf("10.66.%d.%d/32", n/256, n%256)
}
