package analytics

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// The receiving half of the roster - the sweep that keeps a prospect from carrying a trait row, and the delete that drops a customer's traits with them
// These push entries the way core's job would and drive the sweep, the event push and the delete
type rosterEnv struct {
	rec       *Roster
	sigConn   *sql.DB
	channel   uuid.UUID
	customers []RosterEntry // what core would page across on the next sweep
	ctx       context.Context
}

func rosterFixture(t *testing.T) *rosterEnv {
	t.Helper()
	sigConn := testdb.RequireSignals(t)
	channelID := uuid.New()
	t.Cleanup(func() {
		_, _ = sigConn.Exec(`DELETE FROM customer_roster WHERE channel_id = $1`, channelID)
		_, _ = sigConn.Exec(`DELETE FROM customer_traits WHERE channel_id = $1`, channelID)
	})
	return &rosterEnv{rec: NewRoster(signalsdb.New(sigConn)), sigConn: sigConn, channel: channelID, ctx: context.Background()}
}

// customer is one core holds - on the roster once the next sweep or push carries it across
func (e *rosterEnv) customer(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	e.customers = append(e.customers, RosterEntry{
		CustomerID: id, ChannelID: e.channel, Email: "roster-" + uuid.NewString()[:8] + "@example.test", IsActive: true,
	})
	return id
}

func (e *rosterEnv) entry(id uuid.UUID) *RosterEntry {
	for i := range e.customers {
		if e.customers[i].CustomerID == id {
			return &e.customers[i]
		}
	}
	return nil
}

// forget is core deleting the customer - the next sweep no longer carries them
func (e *rosterEnv) forget(id uuid.UUID) {
	kept := e.customers[:0]
	for _, c := range e.customers {
		if c.CustomerID != id {
			kept = append(kept, c)
		}
	}
	e.customers = kept
}

// push drives the receiver exactly as core's job does: page through what core holds, then sweep
func (e *rosterEnv) push(t *testing.T) {
	t.Helper()
	sync := e.rec.BeginRosterSync()
	e.pages(t, sync)
	if _, _, err := sync.Commit(e.ctx); err != nil {
		t.Fatalf("commit roster: %v", err)
	}
}

func (e *rosterEnv) pages(t *testing.T, sync *RosterSync) {
	t.Helper()
	if err := sync.Page(e.ctx, append([]RosterEntry(nil), e.customers...)); err != nil {
		t.Fatalf("page: %v", err)
	}
}

func (e *rosterEnv) onRoster(t *testing.T, id uuid.UUID) bool {
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

	e.push(t)
	if !e.onRoster(t, a) || !e.onRoster(t, b) {
		t.Fatal("a never-ordered customer is missing from the roster, so they would get no trait row at all")
	}
}

func TestADeletedCustomerLosesTheirTraits(t *testing.T) {
	e := rosterFixture(t)
	doomed, survivor := e.customer(t), e.customer(t)

	e.push(t)
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

	e.forget(doomed)
	e.push(t)

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

// The sweep is what makes a push destructive, so a stream that dies partway must not reach it
func TestAPartialPushPrunesNothing(t *testing.T) {
	e := rosterFixture(t)
	a, b := e.customer(t), e.customer(t)
	e.push(t)
	if !e.onRoster(t, a) || !e.onRoster(t, b) {
		t.Fatal("the first push did not land")
	}

	// A push that carries nobody and is abandoned before Commit - what a broken stream looks like
	abandoned := e.rec.BeginRosterSync()
	if err := abandoned.Page(e.ctx, nil); err != nil {
		t.Fatalf("empty page: %v", err)
	}

	if !e.onRoster(t, a) || !e.onRoster(t, b) {
		t.Error("an abandoned push removed customers from the roster")
	}
}

// And the other side of it: a push that completes with nobody in it does sweep, since core saying
// "no customers" is an answer rather than a failure
func TestACompletedEmptyPushSweeps(t *testing.T) {
	e := rosterFixture(t)
	a := e.customer(t)
	e.push(t)
	if !e.onRoster(t, a) {
		t.Fatal("the first push did not land")
	}

	empty := e.rec.BeginRosterSync()
	if _, pruned, err := empty.Commit(e.ctx); err != nil {
		t.Fatalf("commit empty: %v", err)
	} else if pruned == 0 {
		t.Error("a completed empty push pruned nobody, so a customer deleted in core would keep their traits forever")
	}
	if e.onRoster(t, a) {
		t.Error("a completed empty push left a customer on the roster")
	}
}

// marketingRow reads back the fields a campaign filters and addresses on
func (e *rosterEnv) marketingRow(t *testing.T, id uuid.UUID) (email, firstName string, active, optedIn bool, group uuid.NullUUID) {
	t.Helper()
	if err := e.sigConn.QueryRow(
		`SELECT email, first_name, is_active, marketing_opted_in, customer_group_id FROM customer_roster WHERE customer_id = $1`,
		id).Scan(&email, &firstName, &active, &optedIn, &group); err != nil {
		t.Fatalf("read roster row: %v", err)
	}
	return
}

// The sweep is what repairs drift, so every field a send depends on has to land as it was carried - a
// roster that kept identity alone would leave a campaign with nobody to address
func TestTheSweepLandsWhatASendNeeds(t *testing.T) {
	e := rosterFixture(t)
	id := e.customer(t)
	c := e.entry(id)
	c.FirstName, c.MarketingOptedIn = "Ada", true
	c.GroupID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	e.push(t)

	email, firstName, active, optedIn, group := e.marketingRow(t, id)
	if email != c.Email {
		t.Errorf("email = %q, want %q", email, c.Email)
	}
	if firstName != "Ada" {
		t.Errorf("first name = %q, want Ada", firstName)
	}
	if !active {
		t.Error("an active customer landed as inactive, so every audience would skip them")
	}
	if !optedIn {
		t.Error("an opted-in customer landed as opted out, so they would never be mailed")
	}
	if !group.Valid || group.UUID != c.GroupID.UUID {
		t.Errorf("group = %v, want %s - a group audience filters on it", group, c.GroupID.UUID)
	}
}

// Between sweeps the event push is the only thing standing between an unsubscribe and a send
func TestAnUnsubscribeLandsWithoutWaitingForTheSweep(t *testing.T) {
	e := rosterFixture(t)
	id := e.customer(t)
	e.entry(id).MarketingOptedIn = true
	e.push(t)
	if _, _, _, optedIn, _ := e.marketingRow(t, id); !optedIn {
		t.Fatal("the customer did not start opted in, so this proves nothing")
	}

	c := *e.entry(id)
	c.MarketingOptedIn = false
	if err := e.rec.UpsertRosterEntry(e.ctx, c); err != nil {
		t.Fatalf("push the change: %v", err)
	}

	if _, _, _, optedIn, _ := e.marketingRow(t, id); optedIn {
		t.Error("the roster still reads opted in after an unsubscribe was pushed")
	}
}

// A push landing while a sweep is already running has to outlive it. The sweep prunes everything
// stamped before it started, and a customer who registered mid-push was never in the pages it read
func TestAPushDuringASweepIsNotSweptAway(t *testing.T) {
	e := rosterFixture(t)
	existing := e.customer(t)
	e.push(t)

	sweep := e.rec.BeginRosterSync()
	e.pages(t, sweep) // reads core as it was, before the registration below

	latecomer := e.customer(t)
	if err := e.rec.UpsertRosterEntry(e.ctx, *e.entry(latecomer)); err != nil {
		t.Fatalf("push the latecomer: %v", err)
	}

	if _, _, err := sweep.Commit(e.ctx); err != nil {
		t.Fatalf("commit sweep: %v", err)
	}
	if !e.onRoster(t, existing) {
		t.Error("the sweep pruned a customer it carried itself")
	}
	if !e.onRoster(t, latecomer) {
		t.Error("the sweep pruned a customer who registered while it was running, so they are unreachable until the next one")
	}
}

// A customer core dropped has to go before the sweep would have caught them, or a campaign scheduled
// in between mails an account that no longer exists
func TestADeletedCustomerLeavesImmediately(t *testing.T) {
	e := rosterFixture(t)
	id := e.customer(t)
	e.push(t)
	if !e.onRoster(t, id) {
		t.Fatal("the customer never landed, so this proves nothing")
	}

	if err := e.rec.DeleteRosterEntry(e.ctx, id); err != nil {
		t.Fatalf("delete entry: %v", err)
	}
	if e.onRoster(t, id) {
		t.Error("the roster still holds a customer core deleted")
	}
}

// An SMS node runs where core's customers table is not reachable, so the number and the consent that
// governs it have to arrive on the roster or the node has nothing to send on
func TestThePushCarriesWhatAnSMSNodeNeeds(t *testing.T) {
	e := rosterFixture(t)
	id := e.customer(t)
	c := e.entry(id)
	c.Phone, c.SMSOptedIn = "+15551234567", true
	e.push(t)

	var phone string
	var smsOK, emailOK bool
	if err := e.sigConn.QueryRow(
		`SELECT phone, sms_opted_in, marketing_opted_in FROM customer_roster WHERE customer_id = $1`,
		id).Scan(&phone, &smsOK, &emailOK); err != nil {
		t.Fatalf("read roster row: %v", err)
	}
	if phone != "+15551234567" {
		t.Errorf("phone = %q, want the number core holds", phone)
	}
	if !smsOK {
		t.Error("an sms-opted-in customer landed as opted out, so a text node would skip them")
	}
	if emailOK {
		t.Error("sms consent leaked into the email flag - the two are separate permissions")
	}
}
