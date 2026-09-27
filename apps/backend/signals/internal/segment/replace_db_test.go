package segment

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// A refresh writes only what changed - the member who stays keeps the day they joined
func TestReplaceMembershipIsADiff(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	svc := NewService(repo, txrunner.New(conn), conn, nil)
	slug := "diff-" + uuid.NewString()[:8]
	seg, err := repo.Create(ctx, Segment{Slug: slug, Name: slug, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), seg.ID) })
	stays, leaves, joins := uuid.New(), uuid.New(), uuid.New()
	if err := svc.ReplaceMembership(ctx, seg.ID, []uuid.UUID{stays, leaves}); err != nil {
		t.Fatal(err)
	}
	joined := func(id uuid.UUID) (time.Time, bool) {
		var at time.Time
		err := conn.QueryRow(`SELECT added_at FROM customer_segment_memberships WHERE segment_id = $1 AND customer_id = $2`, seg.ID, id).Scan(&at)
		return at, err == nil
	}
	before, _ := joined(stays)

	if err := svc.ReplaceMembership(ctx, seg.ID, []uuid.UUID{stays, joins}); err != nil {
		t.Fatal(err)
	}
	after, ok := joined(stays)
	if !ok || !after.Equal(before) {
		t.Errorf("the member who stayed: present=%v joined %v then %v - want the original date kept", ok, before, after)
	}
	if _, ok := joined(leaves); ok {
		t.Error("the member who left is still in the segment")
	}
	if _, ok := joined(joins); !ok {
		t.Error("the new member was not added")
	}
	if err := svc.ReplaceMembership(ctx, seg.ID, nil); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountMembers(ctx, seg.ID); n != 0 {
		t.Errorf("an empty refresh left %d members, want the segment emptied", n)
	}
}
