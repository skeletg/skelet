package backup

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// The encrypt pump and decrypt must round-trip, and tampering must fail loudly - the
// whole point over plaintext is that a modified object can't restore quietly
func TestEncryptDecryptRoundTrip(t *testing.T) {
	plain := strings.Repeat("skelet dump bytes ", 4096)
	enc := encryptStream("pass-phrase", strings.NewReader(plain))
	blob, err := io.ReadAll(enc)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	_ = enc.Close()
	if bytes.Contains(blob, []byte("skelet dump")) {
		t.Fatal("ciphertext leaks plaintext")
	}

	dec, err := decryptStream("pass-phrase", bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	got, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("read decrypted: %v", err)
	}
	if string(got) != plain {
		t.Fatal("round-trip mismatch")
	}

	if _, err := decryptStream("wrong", bytes.NewReader(blob)); err == nil {
		t.Fatal("wrong passphrase accepted")
	}

	tampered := append([]byte(nil), blob...)
	tampered[len(tampered)/2] ^= 0xff
	dec, err = decryptStream("pass-phrase", bytes.NewReader(tampered))
	if err == nil {
		if _, err = io.ReadAll(dec); err == nil {
			t.Fatal("tampered ciphertext decrypted cleanly")
		}
	}

	truncated := blob[:len(blob)/2]
	dec, err = decryptStream("pass-phrase", bytes.NewReader(truncated))
	if err == nil {
		if _, err = io.ReadAll(dec); err == nil {
			t.Fatal("truncated ciphertext decrypted cleanly")
		}
	}
}
