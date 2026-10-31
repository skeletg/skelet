// Package secret makes the keys setup writes, in the encodings the backend's own startup checks demand
package secret

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
)

// Hex is n random bytes as hex - the opaque secrets, which only need to be long and unguessable
func Hex(rnd io.Reader, n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rnd, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Key32 is 32 random bytes as padded base64: an AES-256 key for CONFIG_ENCRYPTION_KEY and SIGNALS_ENCRYPTION_KEY,
// and the Ed25519 seed that is CUSTOMER_JWT_PRIVATE_KEY
func Key32(rnd io.Reader) (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rnd, b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// CustomerPublic derives CUSTOMER_JWT_PUBLIC_KEY from the private key's seed, the way core does when it signs
func CustomerPublic(seed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(seed))
	if err != nil || len(raw) != ed25519.SeedSize {
		return "", errors.New("CUSTOMER_JWT_PRIVATE_KEY must be base64 of a 32-byte Ed25519 seed")
	}
	pub := ed25519.NewKeyFromSeed(raw).Public().(ed25519.PublicKey)
	return base64.StdEncoding.EncodeToString(pub), nil
}
