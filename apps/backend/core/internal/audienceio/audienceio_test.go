package audienceio

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// The recipe ad platforms specify - trim, lowercase, then SHA-256 hex. Getting it wrong means every
// identifier misses, and the platform reports a match rate of zero rather than an error
func TestHashIdentifierNormalizes(t *testing.T) {
	want := HashIdentifier("alice@example.com")
	for _, in := range []string{" Alice@Example.com ", "ALICE@EXAMPLE.COM", "alice@example.com "} {
		if got := HashIdentifier(in); got != want {
			t.Errorf("HashIdentifier(%q) = %s, want the same hash as the normalized address", in, got)
		}
	}
	if len(want) != 64 {
		t.Errorf("hash is %d chars, want 64 hex", len(want))
	}
}

func TestHashesSkipsMembersWithNoAddress(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	members := func(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) { return ids, nil }
	emails := func(context.Context, []uuid.UUID) ([]string, error) {
		return []string{"a@example.com", "", "  "}, nil
	}

	got, err := Hashes(context.Background(), members, emails, uuid.Nil, uuid.New())
	if err != nil {
		t.Fatalf("Hashes: %v", err)
	}
	if len(got) != 1 || got[0] != HashIdentifier("a@example.com") {
		t.Errorf("got %v, want only the one real address hashed", got)
	}
}

// An empty segment is not an error, and must not reach the address lookup at all
func TestHashesOfAnEmptySegmentAsksForNoAddresses(t *testing.T) {
	members := func(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) { return nil, nil }
	emails := func(context.Context, []uuid.UUID) ([]string, error) {
		t.Error("addresses were looked up for a segment with no members")
		return nil, nil
	}

	got, err := Hashes(context.Background(), members, emails, uuid.Nil, uuid.New())
	if err != nil || len(got) != 0 {
		t.Errorf("Hashes = %v, %v, want empty and no error", got, err)
	}
}

// A failure resolving membership must not fall through to an empty push, which would clear the audience
func TestHashesSurfacesAMembershipFailure(t *testing.T) {
	boom := errors.New("suite unreachable")
	members := func(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) { return nil, boom }
	emails := func(context.Context, []uuid.UUID) ([]string, error) { return nil, nil }

	if _, err := Hashes(context.Background(), members, emails, uuid.Nil, uuid.New()); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the membership failure - a silent empty result would push an empty audience", err)
	}
}
