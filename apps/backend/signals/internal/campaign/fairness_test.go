package campaign

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type passRepo struct {
	Repository
	sending []Campaign
	passed  []uuid.UUID
}

func (r *passRepo) ListABTestingDue(context.Context) ([]Campaign, error)   { return nil, nil }
func (r *passRepo) ClaimDue(context.Context, int32) ([]Campaign, error)    { return nil, nil }
func (r *passRepo) ListSending(context.Context) ([]Campaign, error)        { return r.sending, nil }
func (r *passRepo) RefreshCounts(context.Context, uuid.UUID) error         { return nil }
func (r *passRepo) CountPending(context.Context, uuid.UUID) (int64, error) { return 1, nil }
func (r *passRepo) ListPending(_ context.Context, id uuid.UUID, _ int32) ([]Recipient, error) {
	r.passed = append(r.passed, id)
	time.Sleep(20 * time.Millisecond)
	return nil, nil
}

func TestASpentBudgetStopsThePassSoTheNextOneRotates(t *testing.T) {
	repo := &passRepo{sending: []Campaign{{ID: uuid.New(), ChannelID: uuid.New()}, {ID: uuid.New(), ChannelID: uuid.New()}, {ID: uuid.New(), ChannelID: uuid.New()}}}
	svc := &Service{repo: repo, stores: anyStore{}, track: emailtrack.NewSigner([]byte("k"))}
	if _, err := svc.Drain(context.Background(), 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(repo.passed) != 1 {
		t.Fatalf("a spent budget still gave %d campaigns a pass, want 1 - the tick overruns and the tail is served last every time", len(repo.passed))
	}
}

func TestTheLeastRecentlyAdvancedCampaignLeadsThePass(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	mk := func(advanced time.Duration) uuid.UUID {
		c, err := repo.Create(ctx, Campaign{ChannelID: uuid.New(), Name: "c", Subject: "s", AudienceKind: AudienceAll, Status: StatusDraft})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`UPDATE email_campaigns SET status = 'sending', scheduled_for = now() - interval '1 day', updated_at = now() - $2::interval WHERE id = $1`, c.ID, advanced.String()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM email_campaigns WHERE id = $1`, c.ID) })
		return c.ID
	}
	justAdvanced := mk(time.Second)
	waiting := mk(time.Hour)
	sending, err := repo.ListSending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	at := map[uuid.UUID]int{}
	for i, c := range sending {
		at[c.ID] = i
	}
	if at[waiting] > at[justAdvanced] {
		t.Error("the campaign that just had a pass leads again, so a campaign behind it waits on every tick")
	}
}
