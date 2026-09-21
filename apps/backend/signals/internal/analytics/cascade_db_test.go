package analytics

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Five foreign keys cannot span the two databases, so a sweep stands in for the cascade
// A run left behind for a deleted customer is not litter - it is a live attempt to mail them

func TestDeletingACustomerTakesTheirFlowRunsAndTargetedRows(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	rec := NewRecorder(signalsdb.New(conn), nil)

	customerID, channelID := uuid.New(), uuid.New()
	var flowID, campaignID uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO flows (channel_id, name, trigger_type) VALUES ($1, 'Test', 'order_placed') RETURNING id`,
		channelID).Scan(&flowID); err != nil {
		t.Fatalf("seed flow: %v", err)
	}
	if err := conn.QueryRow(
		`INSERT INTO email_campaigns (channel_id, name) VALUES ($1, 'Test') RETURNING id`,
		channelID).Scan(&campaignID); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM flows WHERE channel_id = $1`, channelID)
		_, _ = conn.Exec(`DELETE FROM email_campaigns WHERE channel_id = $1`, channelID)
		_, _ = conn.Exec(`DELETE FROM customer_roster WHERE customer_id = $1`, customerID)
	})
	if _, err := conn.Exec(
		`INSERT INTO flow_runs (flow_id, customer_id, current_node_id, status) VALUES ($1, $2, 'trigger', 'waiting')`,
		flowID, customerID); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := conn.Exec(
		`INSERT INTO email_campaign_recipients (campaign_id, customer_id, email) VALUES ($1, $2, 'ada@test')`,
		campaignID, customerID); err != nil {
		t.Fatalf("seed recipient: %v", err)
	}
	if err := rec.UpsertRosterEntry(ctx, RosterEntry{CustomerID: customerID, ChannelID: channelID, Email: "ada@test", IsActive: true}); err != nil {
		t.Fatalf("put the customer on the roster: %v", err)
	}

	count := func(q string, arg any) int {
		var n int
		if err := conn.QueryRow(q, arg).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	if count(`SELECT count(*) FROM flow_runs WHERE customer_id = $1`, customerID) != 1 {
		t.Fatal("the run was pruned before the customer was deleted, so this proves nothing")
	}

	if err := rec.DeleteRosterEntry(ctx, customerID); err != nil {
		t.Fatalf("delete the customer: %v", err)
	}

	if n := count(`SELECT count(*) FROM flow_runs WHERE customer_id = $1`, customerID); n != 0 {
		t.Errorf("flow runs left = %d, want none - a waiting run would still try to mail them", n)
	}
	if n := count(`SELECT count(*) FROM email_campaign_recipients WHERE customer_id = $1`, customerID); n != 0 {
		t.Errorf("targeted rows left = %d, want none", n)
	}
}

// Every table that hung off a channel, so a new one added to the sweep without a seed here shows up
// as a row that outlived its channel rather than as a silent gap
var channelOwned = []struct {
	table string
	seed  string
}{
	{"flows", `INSERT INTO flows (channel_id, name, trigger_type) VALUES ($1, 'Test', 'order_placed')`},
	{"email_campaigns", `INSERT INTO email_campaigns (channel_id, name) VALUES ($1, 'Test')`},
	{"email_library", `INSERT INTO email_library (channel_id, name) VALUES ($1, 'Test')`},
	{"email_blocks", `INSERT INTO email_blocks (channel_id, slug) VALUES ($1, 'test-block')`},
	{"email_templates", `INSERT INTO email_templates (channel_id, key) VALUES ($1, 'order_confirmation')`},
}

func TestRetiringAChannelTakesEverythingThatHungOffIt(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := NewChannelRoster(signalsdb.New(conn))

	keep, retire := uuid.New(), uuid.New()
	t.Cleanup(func() {
		for _, o := range channelOwned {
			_, _ = conn.Exec(`DELETE FROM `+o.table+` WHERE channel_id = ANY($1)`, pqArray(keep, retire))
		}
		_, _ = conn.Exec(`DELETE FROM channel_roster WHERE channel_id = ANY($1)`, pqArray(keep, retire))
	})
	for _, ch := range []uuid.UUID{keep, retire} {
		for _, o := range channelOwned {
			if _, err := conn.Exec(o.seed, ch); err != nil {
				t.Fatalf("seed %s: %v", o.table, err)
			}
		}
	}
	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: keep}, {ID: retire}}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// The operator switches one off, so the next push simply does not carry it
	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: keep}}); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	count := func(table string, ch uuid.UUID) int {
		var n int
		if err := conn.QueryRow(`SELECT count(*) FROM `+table+` WHERE channel_id = $1`, ch).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return n
	}
	for _, o := range channelOwned {
		if n := count(o.table, retire); n != 0 {
			t.Errorf("%s left on the retired channel = %d, want none", o.table, n)
		}
		if n := count(o.table, keep); n != 1 {
			t.Errorf("%s on the live channel = %d, want the one it had - the sweep took too much", o.table, n)
		}
	}
}

// The global template row belongs to no channel, and every channel falls back to it - a sweep keyed
// on "no channel owns this" would take it and leave every shop with nothing to fall back to
func TestTheSweepLeavesTheGlobalTemplateAlone(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := NewChannelRoster(signalsdb.New(conn))

	key := "sweep_probe_" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM email_templates WHERE key = $1`, key) })
	if _, err := conn.Exec(`INSERT INTO email_templates (channel_id, key) VALUES (NULL, $1)`, key); err != nil {
		t.Fatalf("seed the global template: %v", err)
	}

	// A channel has to actually be retired or the sweep never runs and this asserts nothing
	keep, retire := uuid.New(), uuid.New()
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM channel_roster WHERE channel_id = ANY($1)`, pqArray(keep, retire))
	})
	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: keep}, {ID: retire}}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if _, gone, err := r.SyncChannels(ctx, []ChannelEntry{{ID: keep}}); err != nil {
		t.Fatalf("second sync: %v", err)
	} else if gone == 0 {
		t.Fatal("nothing was retired, so the sweep never ran")
	}

	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM email_templates WHERE key = $1`, key).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("global templates left = %d, want it untouched", n)
	}
}

func pqArray(ids ...uuid.UUID) interface{ Value() (driver.Value, error) } {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return pq.Array(out)
}
