package xray

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// RealityKeyPair is an x25519 pair in the encoding xray-core expects.
type RealityKeyPair struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
}

// GenerateRealityKeys mirrors `xray x25519`, so the panel can provision REALITY
// inbounds without shelling out to the binary.
func GenerateRealityKeys() (*RealityKeyPair, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &RealityKeyPair{
		PrivateKey: base64.RawURLEncoding.EncodeToString(priv.Bytes()),
		PublicKey:  base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
	}, nil
}

// GenerateShortIDs returns REALITY shortIds: even-length hex strings of at most
// 16 characters.
func GenerateShortIDs(count int) ([]string, error) {
	if count <= 0 {
		count = 8
	}
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		// Vary the length so the set does not look machine generated.
		size := 1 + i%8
		buf := make([]byte, size)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		out = append(out, hex.EncodeToString(buf))
	}
	return out, nil
}

// GeneratePassword returns a base64 secret suitable for trojan or the
// 2022-blake3 shadowsocks ciphers, which require a 32-byte key.
func GeneratePassword(bytes int) string {
	if bytes <= 0 {
		bytes = 16
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("random source failed: %v", err))
	}
	return base64.StdEncoding.EncodeToString(buf)
}
