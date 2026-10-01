package recommend

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type rig struct {
	t       *testing.T
	ctx     context.Context
	cat     *catalog.Service
	svc     *Service
	pt      catalog.ProductType
	sfx     string
	mu      sync.Mutex
	updated []*catalog.ProductUpdated
}

func newRig(t *testing.T) *rig {
	conn := testdb.Require(t)
	tx := txrunner.New(conn)
	q := db.New(conn)
	bus := events.NewBus()
	r := &rig{t: t, ctx: context.Background(), sfx: uuid.NewString()[:8]}
	r.cat = catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(q, tx), Tx: tx, Bus: bus})
	r.svc = NewService(q, tx, r.cat)
	bus.Subscribe(catalog.EventProductUpdated, func(_ context.Context, e events.Event) error {
		if u, ok := e.(*catalog.ProductUpdated); ok {
			r.mu.Lock()
			r.updated = append(r.updated, u)
			r.mu.Unlock()
		}
		return nil
	})
	pt, err := r.cat.CreateProductType(r.ctx, catalog.CreateProductTypeInput{Slug: "rec-" + r.sfx, Name: "rec"})
	if err != nil {
		t.Fatal(err)
	}
	r.pt = pt
	t.Cleanup(func() {
		like := "%" + r.sfx + "%"
		_, _ = conn.Exec(`DELETE FROM catalog_product_children WHERE parent_id IN (SELECT id FROM catalog_products WHERE sku LIKE $1)`, like)
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE sku LIKE $1`, like)
		_, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE slug LIKE $1`, like)
	})
	return r
}

func (r *rig) product(kind catalog.ProductKind, sku string) catalog.Product {
	r.t.Helper()
	p, err := r.cat.CreateProduct(r.ctx, catalog.CreateProductInput{
		ProductTypeID: r.pt.ID, Kind: kind, SKU: sku + "-" + r.sfx, Name: sku, Locale: "en", DefaultPrice: 1000, DefaultCurrencyCode: "USD",
	})
	if err != nil {
		r.t.Fatalf("create %q: %v", sku, err)
	}
	return p
}

func TestLinkWritesAreBoundedAnnouncedAndNameMissingTargets(t *testing.T) {
	r := newRig(t)
	p, other := r.product(catalog.KindSimple, "LINK"), r.product(catalog.KindSimple, "LINK2")
	if err := r.svc.SetProductLinks(r.ctx, p.ID, LinkRelated, []uuid.UUID{uuid.New()}); !errors.Is(err, catalog.ErrProductNotFound) {
		t.Errorf("unknown linked product = %v, want ErrProductNotFound", err)
	}
	if err := r.svc.SetProductLinks(r.ctx, p.ID, LinkRelated, make([]uuid.UUID, maxProductLinks+1)); !errors.Is(err, catalog.ErrTooManyItems) {
		t.Errorf("oversized link set = %v, want ErrTooManyItems", err)
	}
	if err := r.svc.SetProductLinks(r.ctx, p.ID, "sideways", nil); !errors.Is(err, ErrInvalidLinkKind) {
		t.Errorf("unknown kind = %v, want ErrInvalidLinkKind", err)
	}
	if err := r.svc.SetProductLinks(r.ctx, p.ID, LinkRelated, []uuid.UUID{other.ID, p.ID, other.ID}); err != nil {
		t.Fatalf("set links: %v", err)
	}
	got, err := r.svc.ListProductLinks(r.ctx, p.ID, LinkRelated)
	if err != nil || len(got) != 1 || got[0] != other.ID {
		t.Errorf("links = %v (%v), want just the other product", got, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !slices.ContainsFunc(r.updated, func(e *catalog.ProductUpdated) bool {
		return e.ProductID == p.ID && slices.Contains(e.ChangedFields, "links")
	}) {
		t.Error("a link write published no product.updated")
	}
}

func TestAlsoBoughtRollsVariantsUpAndCountsOrders(t *testing.T) {
	r := newRig(t)
	conn := testdb.Require(t)
	parent := r.product(catalog.KindConfigurable, "AB-PAR")
	v1, v2 := r.product(catalog.KindSimple, "AB-V1"), r.product(catalog.KindSimple, "AB-V2")
	other := r.product(catalog.KindSimple, "AB-O")
	if _, err := conn.Exec(`INSERT INTO catalog_product_children (parent_id, child_id) VALUES ($1, $2), ($1, $3)`, parent.ID, v1.ID, v2.ID); err != nil {
		t.Fatal(err)
	}
	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'rec', 'USD') RETURNING id`, "rec-"+r.sfx).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	var orders []uuid.UUID
	for _, skus := range [][]string{{v1.SKU, v2.SKU, other.SKU}, {v1.SKU, other.SKU}} {
		var o uuid.UUID
		if err := conn.QueryRow(`INSERT INTO orders (currency_code, channel_id, status) VALUES ('USD', $1, 'pending') RETURNING id`, ch).Scan(&o); err != nil {
			t.Fatal(err)
		}
		orders = append(orders, o)
		for _, sku := range skus {
			if _, err := conn.Exec(`INSERT INTO order_lines (order_id, product_name, product_sku, unit_price, quantity, total_price) VALUES ($1, 'x', $2, 1, 1, 1)`, o, sku); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Cleanup(func() {
		for _, o := range orders {
			_, _ = conn.Exec(`DELETE FROM order_lines WHERE order_id = $1`, o)
			_, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, o)
		}
		_, _ = conn.Exec(`DELETE FROM product_also_bought`)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch)
	})
	for range 2 {
		if err := r.svc.RebuildAlsoBought(r.ctx); err != nil {
			t.Fatalf("rebuild: %v", err)
		}
	}
	var score int
	if err := conn.QueryRow(`SELECT score FROM product_also_bought WHERE product_id = $1 AND other_product_id = $2`, other.ID, parent.ID).Scan(&score); err != nil {
		t.Fatalf("other -> parent pair: %v", err)
	}
	if score != 2 {
		t.Errorf("score = %d, want 2 distinct orders (two variants in one order count once)", score)
	}
	var variants int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM product_also_bought WHERE product_id = ANY($1) OR other_product_id = ANY($1)`, fmt.Sprintf("{%s,%s}", v1.ID, v2.ID)).Scan(&variants); err != nil {
		t.Fatal(err)
	}
	if variants != 0 {
		t.Errorf("%d pairs still name a variant instead of its parent", variants)
	}
	ids, err := r.svc.ListAlsoBought(r.ctx, other.ID, 0)
	if err != nil || !slices.Contains(ids, parent.ID) {
		t.Errorf("also bought for other = %v (%v), want the parent", ids, err)
	}
}

type fixedRecs []uuid.UUID

func (f fixedRecs) Recommend(context.Context, uuid.UUID, string, int32) ([]uuid.UUID, bool) {
	return f, len(f) > 0
}

func TestARemoteRecommenderAnswersFirst(t *testing.T) {
	r := newRig(t)
	want := uuid.New()
	r.svc.SetRemoteRecommendations(fixedRecs{want})
	ids, err := r.svc.ListAlsoBought(r.ctx, uuid.New(), 4)
	if err != nil || len(ids) != 1 || ids[0] != want {
		t.Fatalf("also bought = %v (%v), want the remote answer", ids, err)
	}
}
