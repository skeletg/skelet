package download

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func grant(used int32, max *int32, expires *time.Time) Grant {
	return Grant{ID: uuid.New(), DownloadsUsed: used, MaxDownloads: max, ExpiresAt: expires}
}

func i32(v int32) *int32 { return &v }

func TestCanConsume_OK(t *testing.T) {
	if err := canConsume(grant(0, i32(3), nil), time.Now()); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestCanConsume_UnlimitedNoExpiry(t *testing.T) {
	if err := canConsume(grant(999, nil, nil), time.Now()); err != nil {
		t.Fatalf("unlimited grant should always consume: %v", err)
	}
}

func TestCanConsume_LimitReached(t *testing.T) {
	if err := canConsume(grant(3, i32(3), nil), time.Now()); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("got %v, want ErrLimitReached", err)
	}
}

func TestCanConsume_BelowLimit(t *testing.T) {
	if err := canConsume(grant(2, i32(3), nil), time.Now()); err != nil {
		t.Fatalf("2 of 3 used should still consume: %v", err)
	}
}

func TestCanConsume_Expired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	if err := canConsume(grant(0, i32(3), &past), time.Now()); !errors.Is(err, ErrExpired) {
		t.Fatalf("got %v, want ErrExpired", err)
	}
}

func TestCanConsume_NotYetExpired(t *testing.T) {
	future := time.Now().Add(time.Hour)
	if err := canConsume(grant(0, i32(3), &future), time.Now()); err != nil {
		t.Fatalf("unexpired grant should consume: %v", err)
	}
}

func TestNewToken_UniqueAndURLSafe(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		tok, err := newToken()
		if err != nil {
			t.Fatalf("newToken: %v", err)
		}
		if seen[tok] {
			t.Fatalf("duplicate token: %s", tok)
		}
		seen[tok] = true
		for _, c := range tok {
			ok := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
			if !ok {
				t.Fatalf("token %q has non-URL-safe char %q", tok, c)
			}
		}
	}
}
