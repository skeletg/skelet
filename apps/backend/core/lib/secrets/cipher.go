// Package secrets is authenticated symmetric encryption for values that mustn't sit in the database as plaintext, currently secret config values like payment-gateway keys
// Ciphertext is self-describing through a versioned prefix, so reads can tell encrypted values from unencrypted plaintext and a future key rotation can add a version without ambiguity
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Tags an encrypted value - anything lacking it is treated as unencrypted plaintext by Decrypt, so reads stay correct before and after the backfill
const prefix = "enc:v1:"

const keyLen = 32

var (
	ErrKeyLength = fmt.Errorf("secrets: encryption key must decode to %d bytes (AES-256)", keyLen)
	ErrNotForUs  = errors.New("secrets: value is not encrypted with this scheme")
)

// Encrypts and decrypts strings with AES-256-GCM under a single key
type Cipher struct {
	aead cipher.AEAD
}

// Builds a Cipher from a base64-encoded 32-byte key, accepting standard or raw base64
func NewCipher(b64Key string) (*Cipher, error) {
	key, err := decodeKey(strings.TrimSpace(b64Key))
	if err != nil {
		return nil, err
	}
	if len(key) != keyLen {
		return nil, ErrKeyLength
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

func decodeKey(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	b, err := base64.RawStdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("secrets: decode key: %w", err)
	}
	return b, nil
}

// Seals plaintext into "enc:v1:" plus base64 of nonce, ciphertext and tag - a fresh random nonce per call
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secrets: nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Reverses Encrypt - a value without the scheme prefix returns unchanged, so unencrypted plaintext passes through and reads work before the backfill - Tampered or wrong-key ciphertext errors on GCM auth failure
func (c *Cipher) Decrypt(stored string) (string, error) {
	if !IsEncrypted(stored) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", fmt.Errorf("secrets: decode ciphertext: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", ErrNotForUs
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("secrets: open: %w", err)
	}
	return string(plain), nil
}

func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, prefix)
}
