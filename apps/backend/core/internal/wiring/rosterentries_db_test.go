package wiring

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The sending half of the customer roster: what core resolves per customer is what the suite will address, filter and text on
// A consent read wrong here is a send that should never have gone out
type rosterSource struct {
	q       *db.Queries
	conn    *sql.DB
	channel uuid.UUID
	ctx     context.Context
	t       *testing.T
}

func rosterSourceFixture(t *testing.T) rosterSource {
	t.Helper()
	conn := testdb.Require(t)
	channelID := uuid.New()
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,$3,'USD',true)`,
		channelID, "roster-"+uuid.NewString()[:8], "Roster Source"); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM customers WHERE channel_id = $1`, channelID)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, channelID)
	})
	return rosterSource{q: db.New(conn), conn: conn, channel: channelID, ctx: context.Background(), t: t}
}

func (s rosterSource) customer() uuid.UUID {
	s.t.Helper()
	c, err := s.q.CreateCustomer(s.ctx, db.CreateCustomerParams{
		Email: "roster-" + uuid.NewString()[:8] + "@example.test", ChannelID: s.channel,
	})
	if err != nil {
		s.t.Fatalf("seed customer: %v", err)
	}
	return c.ID
}

func (s rosterSource) consent(id uuid.UUID, kind string, optedIn bool) {
	s.t.Helper()
	if _, err := s.conn.Exec(`INSERT INTO customer_consents (customer_id, kind, opted_in, source) VALUES ($1, $2, $3, 'profile')`,
		id, kind, optedIn); err != nil {
		s.t.Fatalf("consent %s=%v: %v", kind, optedIn, err)
	}
}

// swept is the customer as the 6-hourly sweep pages them, pushed is as the event push re-reads them -
// the same answer has to come from both, or a sweep would undo what an unsubscribe just pushed
func (s rosterSource) swept(id uuid.UUID) db.CustomerRosterForSyncRow {
	s.t.Helper()
	for off := int32(0); ; off += 5000 {
		rows, err := s.q.CustomerRosterForSync(s.ctx, db.CustomerRosterForSyncParams{Lim: 5000, Off: off})
		if err != nil {
			s.t.Fatalf("page roster: %v", err)
		}
		for _, r := range rows {
			if r.CustomerID == id {
				return r
			}
		}
		if len(rows) < 5000 {
			s.t.Fatalf("customer %s is not on the sweep", id)
		}
	}
}

func (s rosterSource) pushed(id uuid.UUID) db.CustomerRosterEntryRow {
	s.t.Helper()
	row, err := s.q.CustomerRosterEntry(s.ctx, id)
	if err != nil {
		s.t.Fatalf("read entry: %v", err)
	}
	return row
}

// Marketing consent is one kind among several, and transactional_email is Required with DefaultOptIn -
// so a customer who consented to nothing but receipts must still read as opted out of marketing
func TestOnlyMarketingConsentCountsAsMarketingConsent(t *testing.T) {
	s := rosterSourceFixture(t)
	none, transactionalOnly, marketing := s.customer(), s.customer(), s.customer()
	s.consent(transactionalOnly, "transactional_email", true)
	s.consent(marketing, "marketing_email", true)

	if s.swept(none).MarketingOptedIn || s.pushed(none).MarketingOptedIn {
		t.Error("a customer with no consent row at all reads as opted in")
	}
	if s.swept(transactionalOnly).MarketingOptedIn || s.pushed(transactionalOnly).MarketingOptedIn {
		t.Error("consent to receipts was read as consent to marketing, so they would be mailed without ever agreeing")
	}
	if !s.swept(marketing).MarketingOptedIn || !s.pushed(marketing).MarketingOptedIn {
		t.Error("an opted-in customer reads as opted out, so they would never be mailed")
	}
}

// The two consents are separate permissions, and an SMS node has to see the number as well as the answer
func TestSMSConsentAndPhoneTravelSeparatelyFromEmail(t *testing.T) {
	s := rosterSourceFixture(t)
	id := s.customer()
	if _, err := s.conn.Exec(`UPDATE customers SET phone = '+15551234567' WHERE id = $1`, id); err != nil {
		t.Fatalf("give the customer a number: %v", err)
	}
	s.consent(id, "marketing_sms", true)

	for name, got := range map[string]struct {
		phone      string
		sms, email bool
	}{
		"sweep": {phone: s.swept(id).Phone, sms: s.swept(id).SmsOptedIn, email: s.swept(id).MarketingOptedIn},
		"push":  {phone: s.pushed(id).Phone, sms: s.pushed(id).SmsOptedIn, email: s.pushed(id).MarketingOptedIn},
	} {
		if got.phone != "+15551234567" {
			t.Errorf("%s: phone = %q, want the number core holds", name, got.phone)
		}
		if !got.sms {
			t.Errorf("%s: an sms-opted-in customer reads as opted out, so a text node would skip them", name)
		}
		if got.email {
			t.Errorf("%s: sms consent leaked into the email flag", name)
		}
	}
}
