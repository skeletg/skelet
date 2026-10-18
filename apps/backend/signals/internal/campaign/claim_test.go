package campaign

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

// The claim is what stops a campaign re-mailing someone: the send loop takes it BEFORE handing the message to the sender, so a dropped write costs one unsent email instead of re-sending on every later tick - gated on DATABASE_URL because the guarantee lives in the SQL (the status guard + rows-affected), not in Go
func TestClaimForSendIsExactlyOnce(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := &repo{q: db.New(conn)}

	// Channels are core's - the suite carries the id with no table of its own to check it against, so any id will do
	channelID := uuid.New()
	var campaignID, recipientID uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO email_campaigns (channel_id, name, status) VALUES ($1, 'claim-test', 'sending') RETURNING id`,
		channelID).Scan(&campaignID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(`DELETE FROM email_campaigns WHERE id = $1`, campaignID) }()
	if err := conn.QueryRow(
		`INSERT INTO email_campaign_recipients (campaign_id, email) VALUES ($1, 'claim@test.invalid') RETURNING id`,
		campaignID).Scan(&recipientID); err != nil {
		t.Fatal(err)
	}

	first, err := r.ClaimForSend(ctx, recipientID)
	if err != nil || !first {
		t.Fatalf("first claim = %v, err = %v, want true", first, err)
	}
	second, err := r.ClaimForSend(ctx, recipientID) // a concurrent sweep must NOT own it - that false is what a duplicate send would have looked like
	if err != nil {
		t.Fatal(err)
	}
	if second {
		t.Fatal("second claim succeeded - the same recipient would be mailed twice")
	}

	// A send failure releases the claim to 'failed' with its reason, so the recipient never reads as delivered and stops being retried
	if err := r.ReleaseClaimFailed(ctx, recipientID, "smtp refused"); err != nil {
		t.Fatal(err)
	}
	var status, errMsg string
	var sentAt sql.NullTime
	if err := conn.QueryRow(
		`SELECT status, error, sent_at FROM email_campaign_recipients WHERE id = $1`,
		recipientID).Scan(&status, &errMsg, &sentAt); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || errMsg != "smtp refused" || sentAt.Valid {
		t.Fatalf("after release: status=%q error=%q sent_at=%v, want failed/smtp refused/NULL", status, errMsg, sentAt)
	}
}

// Loses every claim, as if a concurrent sweep owned each recipient
type lostClaimRepo struct {
	sendingRepo
	pending []Recipient
}

func (r *lostClaimRepo) ListPending(context.Context, uuid.UUID, int32) ([]Recipient, error) {
	p := r.pending
	r.pending = nil
	return p, nil
}
func (r *lostClaimRepo) ClaimForSend(context.Context, uuid.UUID) (bool, error) { return false, nil }

func TestALostClaimLogsNoSend(t *testing.T) {
	custID := uuid.New()
	repo := &lostClaimRepo{sendingRepo: sendingRepo{&fakeCampaignRepo{}}, pending: []Recipient{{ID: uuid.New(), CustomerID: &custID, Email: "a@example.test"}}}
	rec := &fakeRecorder{}
	sender := &capturingSender{}
	svc := &Service{
		repo: repo, tmpl: emailtemplate.NewService(nil, nil), stores: anyStore{}, sender: sender, recorder: rec,
		contacts: &fakeAudience{optedIn: map[uuid.UUID]bool{custID: true}}, track: emailtrack.NewSigner([]byte("k")),
	}
	if _, err := svc.send(context.Background(), Campaign{ID: uuid.New(), ChannelID: uuid.New(), Subject: "Spring"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(rec.sent) != 0 || len(sender.sent) != 0 {
		t.Errorf("logged %d sends and mailed %d for a recipient another sweep owned - the A/B stats would count a send that never happened", len(rec.sent), len(sender.sent))
	}
}

// A cancel or delete lands mid-pass - the claim re-reads the campaign, so the pass stops at its next recipient instead of
// mailing out its remaining budget
func TestAStoppedCampaignClaimsNoMoreRecipients(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := &repo{q: db.New(conn)}
	for name, stop := range map[string]string{
		"cancelled": `UPDATE email_campaigns SET status = 'cancelled' WHERE id = $1`,
		"deleted":   `UPDATE email_campaigns SET deleted_at = NOW() WHERE id = $1`,
	} {
		var campaignID, recipientID uuid.UUID
		if err := conn.QueryRow(`INSERT INTO email_campaigns (channel_id, name, status) VALUES ($1, 'stop-test', 'sending') RETURNING id`, uuid.New()).Scan(&campaignID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM email_campaigns WHERE id = $1`, campaignID) })
		if err := conn.QueryRow(`INSERT INTO email_campaign_recipients (campaign_id, email) VALUES ($1, 'stop@test.invalid') RETURNING id`, campaignID).Scan(&recipientID); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(stop, campaignID); err != nil {
			t.Fatal(err)
		}
		if claimed, err := r.ClaimForSend(ctx, recipientID); err != nil || claimed {
			t.Errorf("%s campaign: claim = %v, %v - want no send", name, claimed, err)
		}
	}
}

// A campaign in its A/B measurement window must be cancellable, or its held-back audience still gets the winner
func TestAnABTestCanBeCancelledAndADeletedOneNeverResolves(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := &repo{q: db.New(conn)}
	insert := func(deleted bool) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO email_campaigns (channel_id, name, status, ab_enabled, ab_subject_b, ab_test_minutes, ab_test_started_at, deleted_at)
			VALUES ($1, 'ab', 'testing', TRUE, 'B', 1, NOW() - interval '1 hour', CASE WHEN $2 THEN NOW() END) RETURNING id`, uuid.New(), deleted).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM email_campaigns WHERE id = $1`, id) })
		return id
	}
	live, gone := insert(false), insert(true)
	due, err := r.ListABTestingDue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range due {
		if c.ID == gone {
			t.Error("a deleted A/B campaign is still due a winner")
		}
	}
	if ok, err := r.Cancel(ctx, live); err != nil || !ok {
		t.Fatalf("cancel during the test window = %v, %v - want cancelled", ok, err)
	}
}

type skipRecordingRepo struct {
	lostClaimRepo
	skipped map[uuid.UUID]string
}

func (r *skipRecordingRepo) MarkSkipped(_ context.Context, id uuid.UUID, msg string) error {
	r.skipped[id] = msg
	return nil
}

// A recipient with no account has no consent record and no unsubscribe link, so a marketing send must pass them by
func TestARecipientWithoutAnAccountIsNeverMailed(t *testing.T) {
	guest := Recipient{ID: uuid.New(), Email: "guest@example.test"}
	repo := &skipRecordingRepo{sendingRepo: sendingRepo{&fakeCampaignRepo{}}, pending: []Recipient{guest}, skipped: map[uuid.UUID]string{}}
	sender := &capturingSender{}
	svc := &Service{
		repo: repo, tmpl: emailtemplate.NewService(nil, nil), stores: anyStore{}, sender: sender, recorder: &fakeRecorder{},
		contacts: &fakeAudience{optedIn: map[uuid.UUID]bool{}}, track: emailtrack.NewSigner([]byte("k")),
	}
	if _, err := svc.send(context.Background(), Campaign{ID: uuid.New(), ChannelID: uuid.New(), Subject: "Spring"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, ok := repo.skipped[guest.ID]; !ok || len(sender.sent) != 0 {
		t.Errorf("guest recipient: skipped=%v, mailed %d - want skipped and never mailed", repo.skipped, len(sender.sent))
	}
}
