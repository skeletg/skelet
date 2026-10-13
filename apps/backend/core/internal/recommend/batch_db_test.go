package recommend

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type countingRecs struct {
	mu     sync.Mutex
	asked  []uuid.UUID
	limits []int32
}

func (c *countingRecs) Recommend(_ context.Context, id uuid.UUID, _ string, limit int32) ([]uuid.UUID, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.asked = append(c.asked, id)
	c.limits = append(c.limits, limit)
	return []uuid.UUID{uuid.New()}, true
}

// A 0 limit reached the remote provider as "no cap", and its whole list rendered
func TestTheRemoteRecommenderGetsAClampedLimit(t *testing.T) {
	remote := &countingRecs{}
	svc := NewService(nil, nil, nil)
	svc.SetRemoteRecommendations(remote)
	for _, asked := range []int32{0, 10_000} {
		if _, err := svc.ListAlsoBought(context.Background(), uuid.New(), asked); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(remote.limits, []int32{AlsoBoughtDefaultLimit, alsoBoughtFetchMax}) {
		t.Errorf("remote saw limits %v, want %d then %d", remote.limits, AlsoBoughtDefaultLimit, alsoBoughtFetchMax)
	}
}

// One read per source for the whole cart, and a remote recommender is only asked about the newest lines
func TestCartRailsReadEveryProductAtOnce(t *testing.T) {
	r := newRig(t)
	conn := testdb.Require(t)
	a, b, x, y := r.product(catalog.KindSimple, "CR-A"), r.product(catalog.KindSimple, "CR-B"), r.product(catalog.KindSimple, "CR-X"), r.product(catalog.KindSimple, "CR-Y")
	if err := r.svc.SetProductLinks(r.ctx, a.ID, LinkCrosssell, []uuid.UUID{y.ID, x.ID}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.SetProductLinks(r.ctx, b.ID, LinkCrosssell, []uuid.UUID{x.ID}); err != nil {
		t.Fatal(err)
	}
	links, err := r.svc.CrosssellsFor(r.ctx, []uuid.UUID{a.ID, b.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(links[a.ID], []uuid.UUID{y.ID, x.ID}) || !slices.Equal(links[b.ID], []uuid.UUID{x.ID}) {
		t.Errorf("crosssells %v, want each product's links in curated order", links)
	}

	// equal scores come back in a fixed order, not whatever the planner picks
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM product_also_bought WHERE product_id = $1`, a.ID) })
	if _, err := conn.Exec(`INSERT INTO product_also_bought (product_id, other_product_id, score) VALUES ($1, $2, 5), ($1, $3, 5), ($1, $4, 9)`, a.ID, x.ID, y.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	ab, err := r.svc.AlsoBoughtFor(r.ctx, []uuid.UUID{a.ID}, 3)
	if err != nil {
		t.Fatal(err)
	}
	tied := []uuid.UUID{x.ID, y.ID}
	slices.SortFunc(tied, func(p, q uuid.UUID) int { return slices.Compare(p[:], q[:]) })
	if !slices.Equal(ab[a.ID], append([]uuid.UUID{b.ID}, tied...)) {
		t.Errorf("also bought %v, want the top score then the tie broken by id", ab[a.ID])
	}

	remote := &countingRecs{}
	r.svc.SetRemoteRecommendations(remote)
	cart := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	if _, err := r.svc.AlsoBoughtFor(r.ctx, cart, 4); err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(remote.asked, func(p, q uuid.UUID) int { return slices.Compare(p[:], q[:]) })
	newest := slices.Clone(cart[len(cart)-cartRemoteLookups:])
	slices.SortFunc(newest, func(p, q uuid.UUID) int { return slices.Compare(p[:], q[:]) })
	if !slices.Equal(remote.asked, newest) {
		t.Errorf("remote asked about %d products, want only the %d newest", len(remote.asked), cartRemoteLookups)
	}
}

// Orders past the lookback no longer score - the self-join otherwise grows with every order ever kept
func TestTheRebuildIgnoresOrdersPastTheLookback(t *testing.T) {
	r := newRig(t)
	conn := testdb.Require(t)
	a, b := r.product(catalog.KindSimple, "LB-A"), r.product(catalog.KindSimple, "LB-B")
	var ch, o uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'rec', 'USD') RETURNING id`, "lb-"+r.sfx).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO orders (currency_code, channel_id, status, created_at) VALUES ('USD', $1, 'pending', $2) RETURNING id`, ch, time.Now().Add(-2*alsoBoughtLookback)).Scan(&o); err != nil {
		t.Fatal(err)
	}
	for _, sku := range []string{a.SKU, b.SKU} {
		if _, err := conn.Exec(`INSERT INTO order_lines (order_id, product_name, product_sku, unit_price, quantity, total_price) VALUES ($1, 'x', $2, 1, 1, 1)`, o, sku); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM order_lines WHERE order_id = $1`, o)
		_, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, o)
		_, _ = conn.Exec(`DELETE FROM product_also_bought`)
	})
	if err := r.svc.RebuildAlsoBought(r.ctx); err != nil {
		t.Fatal(err)
	}
	if ids, err := r.svc.ListAlsoBought(r.ctx, a.ID, 8); err != nil || len(ids) != 0 {
		t.Errorf("an order two lookbacks old still scored: %v, %v", ids, err)
	}
}
