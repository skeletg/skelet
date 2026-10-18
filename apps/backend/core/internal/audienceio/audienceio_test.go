package audienceio

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
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

// One person can hold several customer records - registered in two channels, or twice with different capitalisation - and each should reach the platform once
func TestHashesListsEachPersonOnce(t *testing.T) {
	members := func(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) {
		return []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}, nil
	}
	emails := func(context.Context, []uuid.UUID) ([]string, error) {
		return []string{"Alice@Example.com", " alice@example.com", "bob@example.com"}, nil
	}
	got, err := Hashes(context.Background(), members, emails, uuid.Nil, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != HashIdentifier("alice@example.com") || got[1] != HashIdentifier("bob@example.com") {
		t.Errorf("got %v, want alice then bob, once each", got)
	}
}

func TestHashesFetchesAddressesInBatches(t *testing.T) {
	ids := make([]uuid.UUID, 2*emailBatch+1)
	for i := range ids {
		ids[i] = uuid.New()
	}
	members := func(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) { return ids, nil }
	asked := map[uuid.UUID]int{}
	calls := 0
	emails := func(_ context.Context, batch []uuid.UUID) ([]string, error) {
		calls++
		if len(batch) > emailBatch {
			t.Errorf("a batch of %d ids, want at most %d", len(batch), emailBatch)
		}
		out := make([]string, len(batch))
		for i, id := range batch {
			asked[id]++
			out[i] = id.String() + "@example.com"
		}
		return out, nil
	}
	got, err := Hashes(context.Background(), members, emails, uuid.Nil, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(asked) != len(ids) || len(got) != len(ids) {
		t.Errorf("%d calls covering %d of %d ids, %d hashes - want 3 calls, every id once", calls, len(asked), len(ids), len(got))
	}
	for id, n := range asked {
		if n != 1 {
			t.Fatalf("id %s looked up %d times", id, n)
		}
	}
}

func TestExportNeedsAValidSegmentAndActsAsTheQueuingAdmin(t *testing.T) {
	ctx := context.Background()
	segment, actor := uuid.New(), uuid.New()
	var gotActor, gotSegment uuid.UUID
	members := func(_ context.Context, a, s uuid.UUID) ([]uuid.UUID, error) {
		gotActor, gotSegment = a, s
		return []uuid.UUID{uuid.New()}, nil
	}
	emails := func(context.Context, []uuid.UUID) ([]string, error) { return []string{"a@example.com"}, nil }
	res := Resource(members, emails)

	if res.ViewPermission != Permission || Permission != "customers.manage" {
		t.Errorf("export gated on %q, want customers.manage like the customer export", res.ViewPermission)
	}
	for _, bad := range []string{"", "not-a-uuid"} {
		if _, err := res.NewExporter(ctx, dataio.NewOpts(map[string]string{optSegmentID: bad})); err == nil {
			t.Errorf("segment %q accepted", bad)
		}
	}

	opts := dataio.NewOpts(map[string]string{optSegmentID: segment.String()})
	opts.ActorID = actor
	exp, err := res.NewExporter(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exp.Columns(ctx); err != nil {
		t.Fatal(err)
	}
	var rows []string
	if err := exp.Stream(ctx, func(r *dataio.Row) error { rows = append(rows, r.Str("hashed_email")); return nil }); err != nil {
		t.Fatal(err)
	}
	if gotActor != actor || gotSegment != segment {
		t.Errorf("members asked as %s for %s, want %s for %s", gotActor, gotSegment, actor, segment)
	}
	if len(rows) != 1 || rows[0] != HashIdentifier("a@example.com") {
		t.Errorf("rows = %v, want the one hashed address", rows)
	}
}
