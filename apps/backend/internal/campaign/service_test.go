package campaign

import (
	"testing"

	"github.com/google/uuid"
)

func TestUnsubscribeTokenRoundTrip(t *testing.T) {
	s := &Service{unsubKey: []byte("test-secret")}
	id := uuid.New()
	tok := s.signToken(id)

	got, err := s.verifyToken(tok)
	if err != nil {
		t.Fatalf("verify valid token: %v", err)
	}
	if got != id {
		t.Fatalf("round-trip id = %s, want %s", got, id)
	}

	for _, bad := range []string{tok + "x", "garbage", "a.b", "", tok[:len(tok)-1]} { // tampered signature / payload
		if _, err := s.verifyToken(bad); err == nil {
			t.Errorf("verifyToken(%q) accepted a forged/invalid token", bad)
		}
	}

	other := &Service{unsubKey: []byte("other-secret")}
	if _, err := other.verifyToken(tok); err == nil {
		t.Error("token verified under the wrong key")
	}
}

func TestValidAudience(t *testing.T) {
	for _, k := range []string{AudienceAll, AudienceGroup, AudienceSegment} {
		if !validAudience(k) {
			t.Errorf("validAudience(%q) = false", k)
		}
	}
	if validAudience("bogus") {
		t.Error("validAudience accepted an unknown kind")
	}
}
