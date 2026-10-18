package sendlog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Exercises the two send-log retention functions inside one rolled-back transaction, so partitions and rows are untouched afterwards (DDL is transactional in Postgres)
func TestEmailSendRetentionFunctions(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	q := signalsdb.New(tx)

	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -4, 0)
	for i := range 3 {
		if _, err := tx.Exec(`INSERT INTO email_sends (channel_id, source, email, sent_at)
			VALUES ($1, 'campaign', $2, $3)`, uuid.New(), fmt.Sprintf("t%d@example.test", i), month.AddDate(0, 0, i)); err != nil {
			t.Fatalf("seed send: %v", err)
		}
	}

	months, err := q.EmailSendMonthsBefore(ctx, month.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("months: %v", err)
	}
	found := false
	for _, m := range months {
		if m.UTC().Format("2006-01") == month.Format("2006-01") {
			found = true
		}
	}
	if !found {
		t.Fatalf("months %v does not include the seeded %s", months, month.Format("2006-01"))
	}

	n, err := q.DropEmailSendMonth(ctx, month)
	if err != nil {
		t.Fatalf("drop: %v", err)
	}
	if n < 3 {
		t.Fatalf("dropped %d rows, want at least the 3 seeded", n)
	}
	var left int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM email_sends WHERE sent_at >= $1 AND sent_at < $2`,
		month, month.AddDate(0, 1, 0)).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d rows survived the month drop", left)
	}
}

// A month whose sends already sit in DEFAULT gets built around them
func TestEnsurePartitionsAdoptsAMonthStuckInTheDefault(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	q := signalsdb.New(tx)

	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	part := "email_sends_" + next.Format("200601")
	if _, err := tx.Exec(`DROP TABLE IF EXISTS ` + part); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO email_sends (channel_id, source, email, sent_at) VALUES ($1, 'campaign', 'stuck@example.test', $2)`, uuid.New(), next.Add(time.Hour)); err != nil {
		t.Fatalf("seed a send into the default: %v", err)
	}

	if err := q.EnsureEmailSendPartitions(ctx); err != nil {
		t.Fatalf("ensure with a month stuck in the default: %v - partition creation is wedged", err)
	}
	var inPart, inDefault int
	if err := tx.QueryRow(`SELECT count(*) FROM ` + part + ` WHERE email = 'stuck@example.test'`).Scan(&inPart); err != nil {
		t.Fatalf("the month was not created: %v", err)
	}
	if err := tx.QueryRow(`SELECT count(*) FROM email_sends_default WHERE email = 'stuck@example.test'`).Scan(&inDefault); err != nil {
		t.Fatal(err)
	}
	if inPart != 1 || inDefault != 0 {
		t.Errorf("the stuck send: %d in its month, %d still in the default - want it moved", inPart, inDefault)
	}
}
