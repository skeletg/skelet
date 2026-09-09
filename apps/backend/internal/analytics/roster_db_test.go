package analytics

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// Gated on both database markers - the roster replaces the join that gave every prospect a trait row, and the ON DELETE CASCADE that dropped a deleted customer's traits with them

type rosterEnv struct {
	rec     *Recorder
	q       *db.Queries
	conn    *sql.DB
	sigConn *sql.DB
	channel uuid.UUID
	ctx     context.Context
}

func rosterFixture(t *testing.T) rosterEnv {
	t.Helper()
	conn, sigConn := testdb.Require(t), testdb.RequireSignals(t)
	channelID := uuid.New()
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,$3,'USD',true)`,
		channelID, "roster-"+uuid.NewString()[:8], "Roster Test"); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = sigConn.Exec(`DELETE FROM customer_roster WHERE channel_id = $1`, channelID)
		_, _ = sigConn.Exec(`DELETE FROM customer_traits WHERE channel_id = $1`, channelID)
		_, _ = conn.Exec(`DELETE FROM customers WHERE channel_id = $1`, channelID)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, channelID)
	})
	q := db.New(conn)
	return rosterEnv{rec: &Recorder{q: q, qs: signalsdb.New(sigConn)}, q: q, conn: conn, sigConn: sigConn, channel: channelID, ctx: context.Background()}
}

func (e rosterEnv) customer(t *testing.T) uuid.UUID {
	t.Helper()
	c, err := e.q.CreateCustomer(e.ctx, db.CreateCustomerParams{
		Email: "roster-" + uuid.NewString()[:8] + "@example.test", ChannelID: e.channel,
	})
	if err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	return c.ID
}

func (e rosterEnv) onRoster(t *testing.T, id uuid.UUID) bool {
	t.Helper()
	var n int
	if err := e.sigConn.QueryRow(`SELECT count(*) FROM customer_roster WHERE customer_id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("count roster: %v", err)
	}
	return n > 0
}

func TestRosterCarriesEveryCustomerIncludingProspects(t *testing.T) {
	e := rosterFixture(t)
	a, b := e.customer(t), e.customer(t)

	if err := e.rec.SyncRoster(e.ctx); err != nil {
		t.Fatalf("SyncRoster: %v", err)
	}
	if !e.onRoster(t, a) || !e.onRoster(t, b) {
		t.Fatal("a never-ordered customer is missing from the roster, so they would get no trait row at all")
	}
}

func TestADeletedCustomerLosesTheirTraits(t *testing.T) {
	e := rosterFixture(t)
	doomed, survivor := e.customer(t), e.customer(t)

	if err := e.rec.SyncRoster(e.ctx); err != nil {
		t.Fatalf("SyncRoster: %v", err)
	}
	var segID uuid.UUID
	if err := e.sigConn.QueryRow(`INSERT INTO customer_segments (slug, name) VALUES ($1,$2) RETURNING id`,
		"roster-"+uuid.NewString()[:8], "Roster Test").Scan(&segID); err != nil {
		t.Fatalf("seed segment: %v", err)
	}
	t.Cleanup(func() { _, _ = e.sigConn.Exec(`DELETE FROM customer_segments WHERE id = $1`, segID) })
	for _, id := range []uuid.UUID{doomed, survivor} {
		if _, err := e.sigConn.Exec(`INSERT INTO customer_traits (customer_id, channel_id) VALUES ($1, $2)`, id, e.channel); err != nil {
			t.Fatalf("seed traits: %v", err)
		}
		if _, err := e.sigConn.Exec(`INSERT INTO customer_segment_memberships (customer_id, segment_id) VALUES ($1, $2)`, id, segID); err != nil {
			t.Fatalf("seed membership: %v", err)
		}
	}

	if _, err := e.conn.Exec(`DELETE FROM customers WHERE id = $1`, doomed); err != nil {
		t.Fatalf("delete customer: %v", err)
	}
	if err := e.rec.SyncRoster(e.ctx); err != nil {
		t.Fatalf("SyncRoster after delete: %v", err)
	}

	if e.onRoster(t, doomed) {
		t.Error("a deleted customer is still on the roster")
	}
	if !e.onRoster(t, survivor) {
		t.Error("the prune took a customer who is still there")
	}

	// Memberships lost the same cascade traits did, so a deleted customer must leave their audiences too
	var memberships int
	if err := e.sigConn.QueryRow(`SELECT count(*) FROM customer_segment_memberships WHERE customer_id = $1`, doomed).Scan(&memberships); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if memberships != 0 {
		t.Error("a deleted customer is still a member of segments, so they stay in every audience")
	}
	for id, wantTraits := range map[uuid.UUID]bool{doomed: false, survivor: true} {
		var n int
		if err := e.sigConn.QueryRow(`SELECT count(*) FROM customer_traits WHERE customer_id = $1`, id).Scan(&n); err != nil {
			t.Fatalf("count traits: %v", err)
		}
		if (n > 0) != wantTraits {
			t.Errorf("customer %s has %d trait rows, want traits=%v - the cascade this replaces is gone", id, n, wantTraits)
		}
	}
}
