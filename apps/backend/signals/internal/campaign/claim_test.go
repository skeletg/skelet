package campaign

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// The claim is what stops a campaign re-mailing someone: the send loop takes it BEFORE handing the message to the sender, so a dropped write costs one unsent email instead of re-sending on every later tick - gated on DATABASE_URL because the guarantee lives in the SQL (the status guard + rows-affected), not in Go
func TestClaimForSendIsExactlyOnce(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := &repo{q: db.New(conn)}

	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to attach a campaign to: %v", err)
	}
	var campaignID, recipientID uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO email_campaigns (channel_id, name) VALUES ($1, 'claim-test') RETURNING id`,
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
