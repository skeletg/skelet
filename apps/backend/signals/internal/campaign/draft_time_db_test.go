package campaign

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// A draft's send time is a plan: saves keep it, and the pipeline leaves the campaign alone even once it has passed
func TestADraftKeepsTheTimeItPlansToGoOutAndIsNotClaimed(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := NewRepository(db.New(conn))

	planned := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	c, err := r.Create(ctx, Campaign{ChannelID: uuid.New(), Name: "draft-time", AudienceKind: AudienceAll, ABWinnerMetric: ABMetricOpen, ScheduledFor: &planned})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM email_campaigns WHERE id = $1`, c.ID) })
	if c.Status != StatusDraft || c.ScheduledFor == nil || !c.ScheduledFor.Equal(planned) {
		t.Fatalf("created: status %q, scheduled_for %v - want a draft planning %v", c.Status, c.ScheduledFor, planned)
	}

	due, err := r.ClaimDue(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range due {
		if d.ID == c.ID {
			t.Fatal("the pipeline claimed a draft whose planned time had passed")
		}
	}

	later := planned.Add(48 * time.Hour)
	for _, want := range []*time.Time{&later, nil} {
		c.ScheduledFor = want
		if ok, err := r.Update(ctx, c); err != nil || !ok {
			t.Fatalf("update: ok=%v err=%v", ok, err)
		}
		got, _, err := r.Get(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if (want == nil) != (got.ScheduledFor == nil) || (want != nil && !got.ScheduledFor.Equal(*want)) {
			t.Errorf("after saving %v the draft plans %v", want, got.ScheduledFor)
		}
	}
}
