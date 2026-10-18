package segment

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// A rule refresh is a diff done in SQL: joiners land, leavers go, and a member who stays keeps the row the growth chart counts from
func TestARuleRefreshSwapsMembershipInSQL(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()
	ch := uuid.New()
	stays := seedRoster(t, conn, rosterRow{channelID: ch, groupSlug: "vip"})
	joins := seedRoster(t, conn, rosterRow{channelID: ch, groupSlug: "vip"})
	leaves := seedRoster(t, conn, rosterRow{channelID: ch, groupSlug: "basic"})
	repo := NewRepository(db.New(conn))
	svc := NewService(repo, txrunner.New(conn), conn, nil)
	seg, err := svc.Create(ctx, CreateInput{Slug: "vip-" + uuid.NewString()[:8], Name: "VIP", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), seg.ID) })
	if err := repo.BulkAddMembers(ctx, seg.ID, []uuid.UUID{stays, leaves}); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err := conn.QueryRow(`SELECT added_at FROM customer_segment_memberships WHERE segment_id = $1 AND customer_id = $2`, seg.ID, stays).Scan(&before); err != nil {
		t.Fatal(err)
	}
	rules := json.RawMessage(`{"conditions":[{"kind":"group","slug":"vip","channelId":"` + ch.String() + `"}]}`)
	if _, err := svc.SetRules(ctx, seg.ID, rules); err != nil {
		t.Fatal(err)
	}
	n, err := svc.RefreshDynamic(ctx, seg.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := repo.MemberIDs(ctx, seg.ID)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	want := []uuid.UUID{stays, joins}
	slices.SortFunc(want, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	if n != 2 || !slices.Equal(ids, want) {
		t.Fatalf("refresh matched %d, members %v - want the two VIPs and not the leaver", n, ids)
	}
	var after time.Time
	if err := conn.QueryRow(`SELECT added_at FROM customer_segment_memberships WHERE segment_id = $1 AND customer_id = $2`, seg.ID, stays).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Errorf("a member who stayed was re-added (%v -> %v)", before, after)
	}
}

// Experiments and promotions target a segment by slug, so it is fixed at creation and checked for shape and uniqueness
func TestSlugsAreValidUniqueAndFixed(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	svc := NewService(repo, txrunner.New(conn), conn, nil)
	slug := "fixed-" + uuid.NewString()[:8]
	seg, err := svc.Create(ctx, CreateInput{Slug: slug, Name: "Fixed", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), seg.ID) })

	if _, err := svc.Create(ctx, CreateInput{Slug: slug, Name: "Again"}); !errors.Is(err, ErrSlugTaken) {
		t.Errorf("duplicate slug: err = %v, want ErrSlugTaken", err)
	}
	for _, bad := range []string{"", "Has Spaces", "-leading", "UPPER", string(make([]byte, 70))} {
		if _, err := svc.Create(ctx, CreateInput{Slug: bad, Name: "x"}); !errors.Is(err, ErrInvalid) {
			t.Errorf("slug %q: err = %v, want ErrInvalid", bad, err)
		}
	}
	if _, err := svc.Update(ctx, UpdateInput{ID: seg.ID, Slug: "renamed-" + uuid.NewString()[:8], Name: "Fixed"}); !errors.Is(err, ErrSlugImmutable) {
		t.Errorf("rename: err = %v, want ErrSlugImmutable", err)
	}
	if _, err := svc.Update(ctx, UpdateInput{ID: seg.ID, Slug: slug, Name: " "}); !errors.Is(err, ErrInvalid) {
		t.Errorf("blank name: err = %v, want ErrInvalid", err)
	}
	if got, err := svc.Update(ctx, UpdateInput{ID: seg.ID, Slug: slug, Name: "Renamed display"}); err != nil || got.Name != "Renamed display" {
		t.Errorf("a name change = %+v, %v", got, err)
	}
	if _, err := svc.Update(ctx, UpdateInput{ID: uuid.New(), Slug: slug, Name: "x"}); !errors.Is(err, ErrSegmentNotFound) {
		t.Errorf("missing segment: err = %v, want ErrSegmentNotFound", err)
	}
}
