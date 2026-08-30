package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func newTestKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, keyLen)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestNewCipher_KeyValidation(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid 32-byte std base64", newTestKey(t), false},
		{"too short", base64.StdEncoding.EncodeToString([]byte("short")), true},
		{"not base64", "!!!not base64!!!", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCipher(tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewCipher err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	c, err := NewCipher(newTestKey(t))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	for _, plain := range []string{"sk_live_abc123", "", "unicode €£¥ 🔐", strings.Repeat("x", 4096)} {
		enc, err := c.Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", plain, err)
		}
		if !IsEncrypted(enc) {
			t.Errorf("Encrypt(%q) = %q, missing scheme prefix", plain, enc)
		}
		if strings.Contains(enc, plain) && plain != "" {
			t.Errorf("ciphertext contains plaintext for %q", plain)
		}
		got, err := c.Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != plain {
			t.Errorf("round-trip = %q, want %q", got, plain)
		}
	}
}

func TestEncrypt_FreshNonce(t *testing.T) {
	c, _ := NewCipher(newTestKey(t))
	a, _ := c.Encrypt("same")
	b, _ := c.Encrypt("same")
	if a == b {
		t.Error("two encryptions of the same plaintext produced identical ciphertext (nonce reuse)")
	}
}

func TestDecrypt_PlaintextPassthrough(t *testing.T) {
	c, _ := NewCipher(newTestKey(t))
	for _, plain := range []string{"plain-value", "", "true", "123"} {
		got, err := c.Decrypt(plain)
		if err != nil {
			t.Fatalf("Decrypt passthrough %q: %v", plain, err)
		}
		if got != plain {
			t.Errorf("passthrough = %q, want %q", got, plain)
		}
	}
}

func TestDecrypt_TamperDetected(t *testing.T) {
	c, _ := NewCipher(newTestKey(t))
	enc, _ := c.Encrypt("sensitive")
	// Flip a byte in the base64 body - "A" alone flakes 1-in-64 runs, whenever the random nonce already starts with it
	body := strings.TrimPrefix(enc, prefix)
	replacement := "A"
	if body[0] == 'A' {
		replacement = "B"
	}
	tampered := prefix + replacement + body[1:]
	if _, err := c.Decrypt(tampered); err == nil {
		t.Error("expected error decrypting tampered ciphertext, got nil")
	}
}

func TestDecrypt_WrongKeyFails(t *testing.T) {
	c1, _ := NewCipher(newTestKey(t))
	c2, _ := NewCipher(newTestKey(t))
	enc, _ := c1.Encrypt("secret")
	if _, err := c2.Decrypt(enc); err == nil {
		t.Error("expected error decrypting with a different key, got nil")
	}
}
