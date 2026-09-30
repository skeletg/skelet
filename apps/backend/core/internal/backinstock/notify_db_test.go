package backinstock

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type fixedChannel struct{ ch channel.Channel }

func (f fixedChannel) GetByID(context.Context, uuid.UUID) (channel.Channel, error) { return f.ch, nil }

type noFlows struct{}

func (noFlows) OnBackInStock(context.Context, uuid.UUID, uuid.UUID, string, string, string, string) int {
	return 0
}

type customersByEmail map[string]uuid.UUID

func (c customersByEmail) CustomerIDByEmail(_ context.Context, _ uuid.UUID, email string) (uuid.UUID, bool) {
	id, ok := c[email]
	return id, ok
}

// Safe for concurrent notifiers; onSend runs before each send and may fail it
type lockedSender struct {
	mu     sync.Mutex
	sent   []string
	onSend func(to string) error
}

func (l *lockedSender) Send(_ context.Context, m email.Message) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	to := m.To
	if l.onSend != nil {
		if err := l.onSend(to); err != nil {
			return err
		}
	}
	l.sent = append(l.sent, to)
	return nil
}

type fixture struct {
	conn      *sql.DB
	q         *db.Queries
	channelID uuid.UUID
	productID uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	conn := testdb.Require(t)
	f := fixture{conn: conn, q: db.New(conn)}
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&f.channelID); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}
	sku := "bis-" + uuid.NewString()[:8]
	var typeID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, $1) RETURNING id`, sku).Scan(&typeID); err != nil {
		t.Fatalf("seed product type: %v", err)
	}
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, sku).Scan(&f.productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, f.productID, f.channelID); err != nil {
		t.Fatalf("seed listing: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM back_in_stock_requests WHERE product_id = $1`, f.productID)
		_, _ = conn.Exec(`DELETE FROM catalog_product_channel_listings WHERE product_id = $1`, f.productID)
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, f.productID)
		_, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
	})
	return f
}

func (f fixture) service(sender email.Sender, cust customerFinder) *Service {
	s := New(f.q, nil, cust, noFlows{}, fixedChannel{channel.Channel{ID: f.channelID, Name: "Test", PublicURL: "https://shop.test"}}, nil, sender, nil)
	return s
}

func (f fixture) record(t *testing.T, s *Service, addr string, customer *uuid.UUID) {
	t.Helper()
	if err := s.Record(context.Background(), Request{ChannelID: f.channelID, ProductID: f.productID, Email: addr, CustomerID: customer}); err != nil {
		t.Fatalf("record %s: %v", addr, err)
	}
}

func (f fixture) pending(t *testing.T) map[string]db.ListPendingBackInStockRow {
	t.Helper()
	rows, err := f.q.ListPendingBackInStock(context.Background(), db.ListPendingBackInStockParams{ProductID: f.productID, ChannelID: f.channelID})
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]db.ListPendingBackInStockRow, len(rows))
	for _, r := range rows {
		out[r.Email] = r
	}
	return out
}

func TestConcurrentRestocksNotifyEachShopperOnce(t *testing.T) {
	f := newFixture(t)
	sender := &lockedSender{}
	s := f.service(sender, customersByEmail{})
	for _, a := range []string{"a@shop.test", "b@shop.test", "c@shop.test"} {
		f.record(t, s, a, nil)
	}

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { s.notifyChannel(context.Background(), f.productID, f.channelID) })
	}
	wg.Wait()
	if len(sender.sent) != 3 {
		t.Errorf("sent %v, want each of the three shoppers exactly once", sender.sent)
	}
}

// Before the claim, the whole product was marked notified after sending - including whoever signed up in between
func TestASignupDuringASendStaysWaiting(t *testing.T) {
	f := newFixture(t)
	sender := &lockedSender{}
	s := f.service(sender, customersByEmail{})
	f.record(t, s, "early@shop.test", nil)
	sender.onSend = func(string) error {
		if len(sender.sent) == 0 {
			f.record(t, s, "late@shop.test", nil)
		}
		return nil
	}

	s.notifyChannel(context.Background(), f.productID, f.channelID)
	if _, waiting := f.pending(t)["late@shop.test"]; !waiting {
		t.Error("a shopper who signed up mid-send was marked notified without a mail")
	}
}

func TestAFailedSendStaysWaitingForTheNextRestock(t *testing.T) {
	f := newFixture(t)
	sender := &lockedSender{onSend: func(to string) error {
		if to == "down@shop.test" {
			return errors.New("smtp refused")
		}
		return nil
	}}
	s := f.service(sender, customersByEmail{})
	f.record(t, s, "down@shop.test", nil)
	f.record(t, s, "fine@shop.test", nil)

	s.notifyChannel(context.Background(), f.productID, f.channelID)
	waiting := f.pending(t)
	if _, ok := waiting["down@shop.test"]; !ok {
		t.Error("the shopper whose notice failed was marked notified anyway")
	}
	if _, ok := waiting["fine@shop.test"]; ok {
		t.Error("the shopper who was mailed is still waiting")
	}
}

// A signed-in shopper typing another address must not tie that address to their account
func TestRecordLinksTheCustomerOnlyToTheirOwnAddress(t *testing.T) {
	f := newFixture(t)
	owner, other := uuid.New(), uuid.New()
	s := f.service(&lockedSender{}, customersByEmail{"mine@shop.test": owner, "theirs@shop.test": other})
	f.record(t, s, "mine@shop.test", &owner)
	f.record(t, s, "theirs@shop.test", &owner)
	f.record(t, s, "nobody@shop.test", &owner)

	got := f.pending(t)
	if r := got["mine@shop.test"]; !r.CustomerID.Valid || r.CustomerID.UUID != owner {
		t.Errorf("own address linked to %v, want the signed-in customer", r.CustomerID)
	}
	for _, addr := range []string{"theirs@shop.test", "nobody@shop.test"} {
		if r := got[addr]; r.CustomerID.Valid {
			t.Errorf("%s was linked to the signed-in customer", addr)
		}
	}
}
