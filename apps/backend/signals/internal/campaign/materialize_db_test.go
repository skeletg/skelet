package campaign

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

// Fails every insert after the first `allow` batches - a materialization cut off partway through
type cutOffRepo struct {
	Repository
	allow int
}

func (r *cutOffRepo) InsertRecipients(ctx context.Context, id uuid.UUID, rows []NewRecipient) error {
	if r.allow == 0 {
		return errors.New("tick budget spent")
	}
	r.allow--
	return r.Repository.InsertRecipients(ctx, id, rows)
}

type flakyAudience struct {
	*fakeAudience
	fail bool
}

func (f *flakyAudience) ListForChannel(ctx context.Context, channelID, after uuid.UUID, limit int) ([]Contact, error) {
	if f.fail {
		return nil, errors.New("roster unavailable")
	}
	return f.fakeAudience.ListForChannel(ctx, channelID, after, limit)
}

func seedDueCampaign(t *testing.T, conn *sql.DB, channelID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO email_campaigns (channel_id, name, subject, status, scheduled_for) VALUES ($1, 'materialize-test', 'Spring', 'scheduled', NOW() - INTERVAL '1 minute') RETURNING id`,
		channelID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM email_campaigns WHERE id = $1`, id) })
	return id
}

func audienceOf(channelID uuid.UUID, n int) *fakeAudience {
	a := &fakeAudience{optedIn: map[uuid.UUID]bool{}}
	for range n {
		id := uuid.New()
		a.contacts = append(a.contacts, Contact{ID: id, ChannelID: channelID, Email: id.String() + "@example.test", FirstName: "C", Active: true})
		a.optedIn[id] = true
	}
	return a
}

func campaignState(t *testing.T, conn *sql.DB, id uuid.UUID) (status string, recipients, total int, materialized bool) {
	t.Helper()
	var at sql.NullTime
	if err := conn.QueryRow(`SELECT status, total_recipients, materialized_at FROM email_campaigns WHERE id = $1`, id).Scan(&status, &total, &at); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT count(*) FROM email_campaign_recipients WHERE campaign_id = $1`, id).Scan(&recipients); err != nil {
		t.Fatal(err)
	}
	return status, recipients, total, at.Valid
}

func TestAnInterruptedMaterializationIsFinishedNotTruncated(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	old := recipientInsertBatch
	recipientInsertBatch = 2
	t.Cleanup(func() { recipientInsertBatch = old })

	channelID := uuid.New()
	id := seedDueCampaign(t, conn, channelID)
	real := NewRepository(db.New(conn))
	audience := audienceOf(channelID, 5)
	c, _, err := real.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	cut := &Service{repo: &cutOffRepo{Repository: real, allow: 1}, contacts: audience}
	if err := cut.materialize(ctx, c); err == nil {
		t.Fatal("the cut-off run reported success")
	}
	if _, recipients, _, materialized := campaignState(t, conn, id); recipients != 2 || materialized {
		t.Fatalf("after the cut: %d recipients, materialized=%v - want the first batch only, and not marked done", recipients, materialized)
	}

	if err := (&Service{repo: real, contacts: audience}).materialize(ctx, c); err != nil {
		t.Fatalf("the retry: %v", err)
	}
	if _, recipients, total, materialized := campaignState(t, conn, id); recipients != 5 || total != 5 || !materialized {
		t.Errorf("after the retry: %d recipients, total %d, materialized=%v - want all 5, and marked done", recipients, total, materialized)
	}
}

func TestAFailedAudienceIsRetriedNotFinalizedAsSent(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	channelID := uuid.New()
	id := seedDueCampaign(t, conn, channelID)
	audience := &flakyAudience{fakeAudience: audienceOf(channelID, 3), fail: true}
	sender := &capturingSender{}
	svc := &Service{
		repo: NewRepository(db.New(conn)), contacts: audience, tmpl: emailtemplate.NewService(emailtemplate.NewRepository(db.New(conn)), nil),
		stores: anyStore{}, sender: sender, recorder: &fakeRecorder{}, track: emailtrack.NewSigner([]byte("campaign-test-key")),
	}

	if _, err := svc.Drain(ctx, 5*time.Second); err == nil {
		t.Fatal("a drain whose audience failed reported success")
	}
	if status, recipients, _, _ := campaignState(t, conn, id); status != StatusSending || recipients != 0 || len(sender.sent) != 0 {
		t.Fatalf("after the failed tick: status %q, %d recipients, %d mailed - want it still sending, untouched", status, recipients, len(sender.sent))
	}

	audience.fail = false
	if _, err := svc.Drain(ctx, 5*time.Second); err != nil {
		t.Fatalf("the next tick: %v", err)
	}
	if status, recipients, _, _ := campaignState(t, conn, id); status != StatusSent || recipients != 3 || len(sender.sent) != 3 {
		t.Errorf("after the next tick: status %q, %d recipients, %d mailed - want sent to all 3", status, recipients, len(sender.sent))
	}
}

func TestPruneSentRecipientsKeepsRecentAndUnsentCampaigns(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := NewRepository(db.New(conn))
	seed := func(status string, sentAgo time.Duration) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(
			`INSERT INTO email_campaigns (channel_id, name, status, sent_at) VALUES ($1, 'prune-test', $2, NOW() - make_interval(secs => $3)) RETURNING id`,
			uuid.New(), status, sentAgo.Seconds()).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM email_campaigns WHERE id = $1`, id) })
		if _, err := conn.Exec(`INSERT INTO email_campaign_recipients (campaign_id, email) VALUES ($1, 'a@example.test'), ($1, 'b@example.test')`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	old, recent, sending := seed(StatusSent, 800*24*time.Hour), seed(StatusSent, time.Hour), seed(StatusSending, 800*24*time.Hour)

	n, err := r.PruneSentRecipients(ctx, time.Now().Add(-730*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	count := func(id uuid.UUID) int {
		var c int
		_ = conn.QueryRow(`SELECT count(*) FROM email_campaign_recipients WHERE campaign_id = $1`, id).Scan(&c)
		return c
	}
	if n != 2 || count(old) != 0 {
		t.Errorf("pruned %d rows, the old sent campaign keeps %d - want its 2 gone", n, count(old))
	}
	if count(recent) != 2 || count(sending) != 2 {
		t.Errorf("recent kept %d, still-sending kept %d - want 2 and 2", count(recent), count(sending))
	}
}
