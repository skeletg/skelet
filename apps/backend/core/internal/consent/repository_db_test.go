package consent

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type recordingOutbox struct {
	n   int
	err error
}

func (o *recordingOutbox) Enqueue(context.Context, *sql.Tx, events.Event) error {
	o.n++
	return o.err
}

func seedCustomer(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	slug := "consent-" + uuid.NewString()[:8]
	var ch, cust uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels ORDER BY created_at LIMIT 1 RETURNING id`, slug).Scan(&ch); err != nil {
		t.Skipf("no channel to clone currency and locale from: %v", err)
	}
	if err := conn.QueryRow(`INSERT INTO customers (channel_id, email, password_hash, customer_group_id)
		SELECT $1, $2, 'x', id FROM customer_groups ORDER BY is_default DESC LIMIT 1 RETURNING id`,
		ch, slug+"@consent.test").Scan(&cust); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, cust)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch)
	})
	return cust
}

func seedAdmin(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(`INSERT INTO admin_users (email, password_hash, first_name, last_name) VALUES ($1, 'x', 'A', 'B') RETURNING id`,
		"consent-"+uuid.NewString()[:8]+"@admin.test").Scan(&id); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM admin_users WHERE id = $1`, id) })
	return id
}

func TestConsentChangesAreRecordedInTheHistory(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	q := db.New(conn)

	for _, durable := range []bool{false, true} {
		name := "direct"
		if durable {
			name = "outbox"
		}
		t.Run(name, func(t *testing.T) {
			cust, admin := seedCustomer(t, conn), seedAdmin(t, conn)
			svc := NewService(NewRepository(q))
			if durable {
				svc.SetOutbox(&recordingOutbox{}, txrunner.New(conn))
			}
			if _, err := svc.SetConsent(ctx, cust, consentkind.MarketingEmail, true, SourceSignupForm); err != nil {
				t.Fatalf("opt in: %v", err)
			}
			if _, err := svc.SetConsentByAdmin(ctx, cust, consentkind.MarketingEmail, false, &admin); err != nil {
				t.Fatalf("admin opt out: %v", err)
			}
			hist, err := svc.History(ctx, cust, 10)
			if err != nil {
				t.Fatalf("history: %v", err)
			}
			if len(hist) != 2 {
				t.Fatalf("history rows = %d, want 2", len(hist))
			}
			if hist[0].Source != SourceAdmin || hist[0].OptedIn || hist[0].AdminID == nil || *hist[0].AdminID != admin {
				t.Errorf("newest = %+v, want the admin's opt-out naming %s", hist[0], admin)
			}
			if hist[1].Source != SourceSignupForm || !hist[1].OptedIn || hist[1].AdminID != nil {
				t.Errorf("oldest = %+v, want the signup opt-in with no admin", hist[1])
			}
			got, err := svc.Get(ctx, cust, consentkind.MarketingEmail)
			if err != nil || got.OptedIn || got.Source != SourceAdmin {
				t.Errorf("current state = %+v (%v), want the admin's opt-out", got, err)
			}
		})
	}
}

func TestARolledBackChangeLeavesNoHistory(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	cust := seedCustomer(t, conn)
	svc := NewService(NewRepository(db.New(conn)))
	svc.SetOutbox(&recordingOutbox{err: errors.New("outbox down")}, txrunner.New(conn))

	if _, err := svc.SetConsent(ctx, cust, consentkind.MarketingEmail, true, SourceProfile); err == nil {
		t.Fatal("SetConsent succeeded with the outbox failing")
	}
	hist, err := svc.History(ctx, cust, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 0 {
		t.Errorf("history rows = %d after a rolled-back change, want 0", len(hist))
	}
	if c, _ := svc.Get(ctx, cust, consentkind.MarketingEmail); !c.IsDefault {
		t.Errorf("consent row survived the rollback: %+v", c)
	}
}

func TestHistoryOutlivesTheAdmin(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	cust, admin := seedCustomer(t, conn), seedAdmin(t, conn)
	svc := NewService(NewRepository(db.New(conn)))
	if _, err := svc.SetConsentByAdmin(ctx, cust, consentkind.MarketingEmail, true, &admin); err != nil {
		t.Fatalf("admin opt in: %v", err)
	}
	if _, err := conn.Exec(`DELETE FROM admin_users WHERE id = $1`, admin); err != nil {
		t.Fatalf("delete admin: %v", err)
	}
	hist, err := svc.History(ctx, cust, 10)
	if err != nil || len(hist) != 1 {
		t.Fatalf("history = %d rows (%v), want the entry kept", len(hist), err)
	}
	if hist[0].AdminID != nil || hist[0].Source != SourceAdmin {
		t.Errorf("entry = %+v, want source admin with the admin id cleared", hist[0])
	}
}
