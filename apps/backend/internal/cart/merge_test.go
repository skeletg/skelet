package cart

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

// No-op sql driver so tx.Manager can hand out real transactions - the fake repo
// ignores the *sql.Tx entirely, only the Begin/Commit lifecycle has to exist
type noopDriver struct{}
type noopConn struct{}
type noopTx struct{}

func (noopDriver) Open(string) (driver.Conn, error)  { return noopConn{}, nil }
func (noopConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (noopConn) Close() error                        { return nil }
func (noopConn) Begin() (driver.Tx, error)           { return noopTx{}, nil }
func (noopTx) Commit() error                         { return nil }
func (noopTx) Rollback() error                       { return nil }

var registerNoopDriver sync.Once

func newTestTxManager(t *testing.T) *tx.Manager {
	t.Helper()
	registerNoopDriver.Do(func() { sql.Register("cart-noop", noopDriver{}) })
	db, err := sql.Open("cart-noop", "")
	if err != nil {
		t.Fatalf("open noop db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return tx.New(db)
}

// mergeRepo backs MergeGuestCart with recorded writes over the inert fakeRepo
type mergeRepo struct {
	fakeRepo
	carts        map[uuid.UUID]Cart
	linesByCart  map[uuid.UUID][]CartLine
	activeByCust map[uuid.UUID]Cart

	mergedCarts []uuid.UUID
	reassigned  []uuid.UUID
	simple      []AddCartLineInput
	configured  []AddConfiguredLineInput
	bundles     []AddBundleLineInput
}

func (r *mergeRepo) GetCartByID(_ context.Context, id uuid.UUID) (Cart, error) {
	c, ok := r.carts[id]
	if !ok {
		return Cart{}, sql.ErrNoRows
	}
	return c, nil
}

func (r *mergeRepo) GetCartByCustomerAndChannel(_ context.Context, customerID, _ uuid.UUID) (Cart, error) {
	c, ok := r.activeByCust[customerID]
	if !ok {
		return Cart{}, sql.ErrNoRows
	}
	return c, nil
}

func (r *mergeRepo) ListCartLinesByCart(_ context.Context, cartID uuid.UUID) ([]CartLine, error) {
	return r.linesByCart[cartID], nil
}

func (r *mergeRepo) SetCartMerged(_ context.Context, cartID uuid.UUID) error {
	r.mergedCarts = append(r.mergedCarts, cartID)
	return nil
}

func (r *mergeRepo) ReassignCart(_ context.Context, cartID, customerID uuid.UUID) (Cart, error) {
	r.reassigned = append(r.reassigned, cartID)
	c := r.carts[cartID]
	c.CustomerID = &customerID
	r.carts[cartID] = c
	return c, nil
}

func (r *mergeRepo) UpsertCartLineBySku(_ context.Context, _ uuid.UUID, in AddCartLineInput) (CartLine, error) {
	r.simple = append(r.simple, in)
	return CartLine{}, nil
}

func (r *mergeRepo) UpsertConfiguredCartLine(_ context.Context, _ uuid.UUID, in AddConfiguredLineInput) (CartLine, error) {
	r.configured = append(r.configured, in)
	return CartLine{}, nil
}

func (r *mergeRepo) UpsertBundleCartLine(_ context.Context, _ uuid.UUID, in AddBundleLineInput) (CartLine, error) {
	r.bundles = append(r.bundles, in)
	return CartLine{}, nil
}

func (r *mergeRepo) WithTx(*sql.Tx) Repository { return r }

func TestMergeGuestCart(t *testing.T) {
	customerID := uuid.New()
	channelID := uuid.New()
	guestID := uuid.New()
	destID := uuid.New()

	optionID := uuid.New()
	configuredLine := CartLine{
		CartID: guestID, ProductSku: "mug-engraved", ProductName: "Engraved Mug",
		UnitPrice: 1500, Quantity: 2,
		Options: []CartLineOption{{
			ProductOptionID: optionID, InputType: "text",
			OptionTitle: "Engraving", TextValue: "for mom", PriceDelta: 500,
		}},
	}
	simpleLine := CartLine{CartID: guestID, ProductSku: "mug-plain", ProductName: "Plain Mug", UnitPrice: 1000, Quantity: 1}
	bundleLine := CartLine{
		CartID: guestID, ProductSku: "mug-set", ProductName: "Mug Set", UnitPrice: 4000, Quantity: 1,
		Components: []CartLineComponent{{ProductSku: "mug-plain", ProductName: "Plain Mug", UnitPrice: 1000, Quantity: 4}},
	}

	newRepo := func(withDest bool) *mergeRepo {
		r := &mergeRepo{
			carts: map[uuid.UUID]Cart{
				guestID: {ID: guestID, ChannelID: channelID, Status: "active"},
			},
			linesByCart:  map[uuid.UUID][]CartLine{guestID: {configuredLine, simpleLine, bundleLine}},
			activeByCust: map[uuid.UUID]Cart{},
		}
		if withDest {
			dest := Cart{ID: destID, CustomerID: &customerID, ChannelID: channelID, Status: "active"}
			r.carts[destID] = dest
			r.activeByCust[customerID] = dest
		}
		return r
	}

	t.Run("configured lines keep their option snapshots", func(t *testing.T) {
		repo := newRepo(true)
		svc := NewService(repo, newTestTxManager(t), nil)
		if _, err := svc.MergeGuestCart(context.Background(), customerID, guestID); err != nil {
			t.Fatalf("MergeGuestCart: %v", err)
		}
		if len(repo.mergedCarts) != 1 || repo.mergedCarts[0] != guestID {
			t.Fatalf("guest cart not tombstoned, merged=%v", repo.mergedCarts)
		}
		if len(repo.configured) != 1 {
			t.Fatalf("expected 1 configured upsert, got %d (options dropped to the simple path?)", len(repo.configured))
		}
		got := repo.configured[0]
		if got.ProductSku != "mug-engraved" || len(got.Options) != 1 {
			t.Fatalf("configured line arrived mangled: %+v", got)
		}
		if got.Options[0].TextValue != "for mom" || got.Options[0].ProductOptionID != optionID {
			t.Fatalf("option snapshot not cloned verbatim: %+v", got.Options[0])
		}
		if len(repo.simple) != 1 || repo.simple[0].ProductSku != "mug-plain" {
			t.Fatalf("simple line mishandled: %+v", repo.simple)
		}
		if len(repo.bundles) != 1 || len(repo.bundles[0].Components) != 1 {
			t.Fatalf("bundle line mishandled: %+v", repo.bundles)
		}
	})

	t.Run("collisions never clobber the customer's configured lines", func(t *testing.T) {
		repo := newRepo(true)
		// The customer already has: a CONFIGURED mug-engraved and a plain mug-plain
		custConfigured := configuredLine
		custConfigured.CartID = destID
		custPlain := CartLine{CartID: destID, ProductSku: "mug-plain", ProductName: "Plain Mug", UnitPrice: 1000, Quantity: 3}
		repo.linesByCart[destID] = []CartLine{custConfigured, custPlain}
		// The guest brings: a PLAIN mug-engraved (would reset the configured line's price via the SKU upsert),
		// a CONFIGURED mug-plain (would replace the customer's picks), and an uncontested plain line
		guestPlainOnConfigured := CartLine{CartID: guestID, ProductSku: "mug-engraved", ProductName: "Engraved Mug", UnitPrice: 1500, Quantity: 1}
		guestConfiguredOnPlain := custConfigured
		guestConfiguredOnPlain.CartID = guestID
		guestConfiguredOnPlain.ProductSku = "mug-plain"
		fresh := CartLine{CartID: guestID, ProductSku: "mug-new", ProductName: "New Mug", UnitPrice: 900, Quantity: 1}
		repo.linesByCart[guestID] = []CartLine{guestPlainOnConfigured, guestConfiguredOnPlain, fresh}

		svc := NewService(repo, newTestTxManager(t), nil)
		if _, err := svc.MergeGuestCart(context.Background(), customerID, guestID); err != nil {
			t.Fatalf("MergeGuestCart: %v", err)
		}
		if len(repo.configured) != 0 {
			t.Fatalf("guest configured line replaced the customer's plain line: %+v", repo.configured)
		}
		if len(repo.simple) != 1 || repo.simple[0].ProductSku != "mug-new" {
			t.Fatalf("expected only the uncontested line to merge, got %+v", repo.simple)
		}
	})

	t.Run("no active cart adopts the guest cart wholesale", func(t *testing.T) {
		repo := newRepo(false)
		svc := NewService(repo, newTestTxManager(t), nil)
		merged, err := svc.MergeGuestCart(context.Background(), customerID, guestID)
		if err != nil {
			t.Fatalf("MergeGuestCart: %v", err)
		}
		if len(repo.reassigned) != 1 || repo.reassigned[0] != guestID {
			t.Fatalf("expected adoption via ReassignCart, got %v", repo.reassigned)
		}
		// Adoption must not rewrite lines - the rows carry over untouched
		if len(repo.simple)+len(repo.configured)+len(repo.bundles) != 0 {
			t.Fatal("adoption path should not upsert any lines")
		}
		if merged.ID != guestID {
			t.Fatalf("adopted cart id = %s, want the guest cart %s", merged.ID, guestID)
		}
	})

	t.Run("an owned cart is not mergeable", func(t *testing.T) {
		repo := newRepo(true)
		other := uuid.New()
		owned := repo.carts[guestID]
		owned.CustomerID = &other
		repo.carts[guestID] = owned
		svc := NewService(repo, newTestTxManager(t), nil)
		_, err := svc.MergeGuestCart(context.Background(), customerID, guestID)
		if err == nil || !strings.Contains(err.Error(), "not a guest cart") {
			t.Fatalf("merging an owned cart must be rejected, got %v", err)
		}
		if len(repo.mergedCarts) != 0 {
			t.Fatal("owned cart must not be tombstoned")
		}
	})

	t.Run("consumed guest cart is idempotent", func(t *testing.T) {
		repo := newRepo(true)
		spent := repo.carts[guestID]
		spent.Status = "merged"
		repo.carts[guestID] = spent
		svc := NewService(repo, newTestTxManager(t), nil)
		merged, err := svc.MergeGuestCart(context.Background(), customerID, guestID)
		if err != nil {
			t.Fatalf("MergeGuestCart: %v", err)
		}
		if merged.ID != destID {
			t.Fatalf("replay should return the customer's active cart, got %s", merged.ID)
		}
		if len(repo.simple)+len(repo.configured)+len(repo.bundles) != 0 {
			t.Fatal("replay must not re-apply lines")
		}
	})
}
