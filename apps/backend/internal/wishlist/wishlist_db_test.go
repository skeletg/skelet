package wishlist

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// A wishlist is per-customer state reached two ways: the signed-in account, and a public share token
// anyone holding the link can read - so the guarantees are that one customer's writes never touch
// another's list, and that a token resolves to exactly its owner's products
//
// DB-backed because the idempotency is ON CONFLICT DO NOTHING, the ordering is the query's, and the
// share token's uniqueness is an index

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	conn := testdb.Require(t)
	return NewService(dbsqlc.New(conn)), conn
}

// Reuses whatever group and channel the database already has rather than seeding a customer graph
func seedCustomer(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var groupID, channelID uuid.UUID
	if err := conn.QueryRowContext(ctx, `SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Skipf("no customer group to seed against: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}
	var id uuid.UUID
	email := "wishlist-" + uuid.NewString() + "@example.invalid"
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO customers (email, password_hash, customer_group_id, channel_id)
		 VALUES ($1, 'x', $2, $3) RETURNING id`, email, groupID, channelID).Scan(&id); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, id) }) // items and shares cascade
	return id
}

func seedProduct(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var typeID uuid.UUID
	if err := conn.QueryRowContext(ctx, `SELECT id FROM catalog_product_types LIMIT 1`).Scan(&typeID); err != nil {
		t.Skipf("no product type to hang a fixture product off: %v", err)
	}
	suffix := uuid.NewString()[:8]
	var id uuid.UUID
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO catalog_products (product_type_id, sku, url, kind)
		 VALUES ($1, $2, $3, 'simple') RETURNING id`,
		typeID, "WISH-"+suffix, "wish-"+suffix).Scan(&id); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, id) })
	return id
}

func TestSavingTheSameProductTwiceIsOneEntry(t *testing.T) {
	// The storefront merges a guest's client-side list into the account on login, so it re-adds
	// everything already saved - a second row there would double the list on every sign-in
	svc, conn := newTestService(t)
	ctx := context.Background()
	customerID, productID := seedCustomer(t, conn), seedProduct(t, conn)

	for range 3 {
		if err := svc.Add(ctx, customerID, productID); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	ids, err := svc.ListIDs(ctx, customerID)
	if err != nil {
		t.Fatalf("ListIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != productID {
		t.Errorf("list = %v, want exactly one entry for %v", ids, productID)
	}
}

func TestRemovingLeavesEveryOtherCustomerAlone(t *testing.T) {
	// One table holds every customer's list, so a Remove missing its customer_id predicate would
	// unsave the product for everyone who wanted it
	svc, conn := newTestService(t)
	ctx := context.Background()
	mine, theirs := seedCustomer(t, conn), seedCustomer(t, conn)
	productID := seedProduct(t, conn)

	for _, customerID := range []uuid.UUID{mine, theirs} {
		if err := svc.Add(ctx, customerID, productID); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := svc.Remove(ctx, mine, productID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if ids, err := svc.ListIDs(ctx, mine); err != nil || len(ids) != 0 {
		t.Errorf("my list = (%v, %v), want empty", ids, err)
	}
	if ids, err := svc.ListIDs(ctx, theirs); err != nil || len(ids) != 1 {
		t.Errorf("their list = (%v, %v), want the product still saved", ids, err)
	}
}

func TestRemovingSomethingUnsavedIsNotAnError(t *testing.T) { // the storefront toggles optimistically and can double-fire
	svc, conn := newTestService(t)
	if err := svc.Remove(context.Background(), seedCustomer(t, conn), seedProduct(t, conn)); err != nil {
		t.Fatalf("Remove of an unsaved product: %v", err)
	}
}

func TestTheListReadsNewestFirst(t *testing.T) { // the account page shows the most recent save at the top
	svc, conn := newTestService(t)
	ctx := context.Background()
	customerID := seedCustomer(t, conn)
	older, newer := seedProduct(t, conn), seedProduct(t, conn)

	for _, productID := range []uuid.UUID{older, newer} {
		if err := svc.Add(ctx, customerID, productID); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	// Backdates the first save rather than trusting two statements to land on different clock ticks
	if _, err := conn.Exec(
		`UPDATE wishlist_items SET created_at = now() - interval '1 hour' WHERE customer_id = $1 AND product_id = $2`,
		customerID, older,
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	ids, err := svc.ListIDs(ctx, customerID)
	if err != nil {
		t.Fatalf("ListIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != newer || ids[1] != older {
		t.Errorf("list = %v, want [%v %v]", ids, newer, older)
	}
}

func TestShareTokenIsMintedOnceAndStaysStable(t *testing.T) {
	// Shared links live in bookmarks and messages - re-minting on each call would break every link
	// already out in the world
	svc, conn := newTestService(t)
	ctx := context.Background()
	customerID := seedCustomer(t, conn)

	first, err := svc.ShareToken(ctx, customerID)
	if err != nil {
		t.Fatalf("ShareToken: %v", err)
	}
	for range 3 {
		again, err := svc.ShareToken(ctx, customerID)
		if err != nil {
			t.Fatalf("repeat ShareToken: %v", err)
		}
		if again != first {
			t.Fatalf("token changed from %q to %q", first, again)
		}
	}
	var rows int
	if err := conn.QueryRow(`SELECT count(*) FROM wishlist_shares WHERE customer_id = $1`, customerID).Scan(&rows); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if rows != 1 {
		t.Errorf("share rows = %d, want 1", rows)
	}
}

func TestShareTokensAvoidConfusableCharacters(t *testing.T) {
	// The links get read aloud and hand-typed, so i/l/1 and o/0 would turn into support tickets
	svc, conn := newTestService(t)
	ctx := context.Background()

	seen := map[string]bool{}
	for range 5 {
		token, err := svc.ShareToken(ctx, seedCustomer(t, conn))
		if err != nil {
			t.Fatalf("ShareToken: %v", err)
		}
		if seen[token] {
			t.Fatalf("token %q was minted twice", token)
		}
		seen[token] = true
		if len(token) != 16 {
			t.Errorf("token %q is %d chars, want 16", token, len(token))
		}
		for _, c := range token {
			if !strings.ContainsRune(shareAlphabet, c) {
				t.Errorf("token %q holds %q, which is outside the hand-typable alphabet", token, c)
			}
		}
	}
}

func TestAnUnknownShareTokenIsAnEmptyListNotAnError(t *testing.T) {
	// The public share page renders whatever comes back - an error here would 500 on every stale or
	// mistyped link instead of showing an empty wishlist
	svc, _ := newTestService(t)
	ids, err := svc.ResolveShare(context.Background(), "nosuchtoken12345")
	if err != nil {
		t.Fatalf("ResolveShare: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids = %v, want empty", ids)
	}
}

func TestAShareTokenResolvesOnlyItsOwnersProducts(t *testing.T) {
	// The token is the whole authorization for a public read, so resolving it to the wrong list would
	// publish someone else's saved products to anyone holding a link
	svc, conn := newTestService(t)
	ctx := context.Background()
	owner, stranger := seedCustomer(t, conn), seedCustomer(t, conn)
	mine, theirs := seedProduct(t, conn), seedProduct(t, conn)

	if err := svc.Add(ctx, owner, mine); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := svc.Add(ctx, stranger, theirs); err != nil {
		t.Fatalf("Add: %v", err)
	}
	token, err := svc.ShareToken(ctx, owner)
	if err != nil {
		t.Fatalf("ShareToken: %v", err)
	}

	ids, err := svc.ResolveShare(ctx, token)
	if err != nil {
		t.Fatalf("ResolveShare: %v", err)
	}
	if len(ids) != 1 || ids[0] != mine {
		t.Errorf("resolved %v, want only the owner's product %v", ids, mine)
	}
}

func TestSaveAndUnsavePublishForTheActivityTracker(t *testing.T) {
	// Synapse builds product affinity off these, so a silent Add is a customer profile that never
	// learns what they wanted
	svc, conn := newTestService(t)
	ctx := context.Background()
	bus := events.NewBus()
	svc.SetEventBus(bus)
	customerID, productID := seedCustomer(t, conn), seedProduct(t, conn)

	got := make(chan string, 4)
	bus.Subscribe(EventItemAdded, func(context.Context, events.Event) error { got <- EventItemAdded; return nil })
	bus.Subscribe(EventItemRemoved, func(context.Context, events.Event) error { got <- EventItemRemoved; return nil })

	if err := svc.Add(ctx, customerID, productID); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := svc.Remove(ctx, customerID, productID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := bus.WaitIdle(wait); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}
	close(got)
	fired := map[string]bool{}
	for e := range got {
		fired[e] = true
	}
	for _, want := range []string{EventItemAdded, EventItemRemoved} {
		if !fired[want] {
			t.Errorf("%s never fired", want)
		}
	}
}
