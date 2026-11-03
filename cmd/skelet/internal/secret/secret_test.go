package secret

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

// From core itself - ./skelet cli keys customer-public on the seed of bytes 0..31 - so a change in either encoding
// shows up here rather than as a storefront that can't verify a session
const (
	vectorSeed   = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	vectorPublic = "A6EHv/POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg="
)

func TestCustomerPublicMatchesCore(t *testing.T) {
	if got, err := CustomerPublic(vectorSeed); err != nil || got != vectorPublic {
		t.Errorf("CustomerPublic = %q, %v - core says %s", got, err, vectorPublic)
	}
	for _, bad := range []string{"", "not base64!", base64.StdEncoding.EncodeToString(make([]byte, 31))} {
		if _, err := CustomerPublic(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestEncodings(t *testing.T) {
	h, err := Hex(rand.Reader, 32)
	if b, derr := hex.DecodeString(h); err != nil || derr != nil || len(b) != 32 {
		t.Errorf("Hex = %q, %v", h, err)
	}
	k, err := Key32(rand.Reader)
	if b, derr := base64.StdEncoding.DecodeString(k); err != nil || derr != nil || len(b) != 32 {
		t.Errorf("Key32 = %q, %v - core decodes it with padded StdEncoding", k, err)
	}
	if _, err := CustomerPublic(k); err != nil {
		t.Errorf("a Key32 is not a usable customer seed: %v", err)
	}
}
