package e2e

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

var (
	filtersOnce   sync.Once
	filtersPlugin = sdk.NewPluginForTest("e2e-filters")
	taggedRecords sync.Map // tag -> []uuid.UUID, what the plugin's own table would hold
)

// Stands in for a plugin tagging records in its own table - every list matches the records carrying a picked tag
func registerE2EListFilters() {
	filtersOnce.Do(func() {
		match := func(_ context.Context, tags []string) ([]uuid.UUID, error) {
			var out []uuid.UUID
			for _, tag := range tags {
				if ids, ok := taggedRecords.Load(tag); ok {
					out = append(out, ids.([]uuid.UUID)...)
				}
			}
			return out, nil
		}
		opts := []sdk.ListFilterOption{{Value: "flagged", Label: "Flagged"}}
		for _, list := range []sdk.ListKind{sdk.ListProducts, sdk.ListCustomers, sdk.ListOrders} {
			filtersPlugin.RegisterListFilter(sdk.ListFilter{List: list, Key: "tag", Label: "Tag", Options: opts, Match: match})
		}
	})
}

func pickTag(t *testing.T, h *harness, list sdk.ListKind, tag string) ([]uuid.UUID, bool) {
	t.Helper()
	ids, restrict, err := plugin.ListFilterIDs(h.Ctx, list, []plugin.ListFilterPick{{Key: "e2e-filters:tag", Values: []string{tag}}})
	if err != nil {
		t.Fatal(err)
	}
	return ids, restrict
}

// Scenario: a plugin's filter narrows the products, customers and orders lists to the records it matched, counts and
// all, and a filter matching nothing empties the list rather than leaving it whole
func TestAPluginsListFilterNarrowsCoreLists(t *testing.T) {
	h := newHarness(t)
	registerE2EListFilters()
	f := h.NewStore(t, 20, 2000)
	ch := f.Channel.ID
	flag := "flagged-" + uuid.NewString()[:8]

	extra := h.AddProduct(t, f, 1500, true, 5)
	taggedRecords.Store(flag, []uuid.UUID{extra.ID})
	only, restrict := pickTag(t, h, sdk.ListProducts, flag)
	products := func(only []uuid.UUID, restrict bool) []uuid.UUID {
		rows, total, err := h.D.Catalog.ListPageWithGraph(h.Ctx, catalog.ProductPageFilter{ChannelID: &ch, Limit: 50, OnlyIDs: only, RestrictIDs: restrict})
		if err != nil || int(total) != len(rows) {
			t.Fatalf("products: %d rows of %d, %v", len(rows), total, err)
		}
		ids := make([]uuid.UUID, len(rows))
		for i, p := range rows {
			ids[i] = p.ID
		}
		return ids
	}
	if all := products(nil, false); !slices.Contains(all, f.Product.ID) || !slices.Contains(all, extra.ID) {
		t.Fatalf("the unfiltered list %v misses the store's products", all)
	}
	if got := products(only, restrict); !slices.Equal(got, []uuid.UUID{extra.ID}) {
		t.Errorf("filtered products %v, want only %s", got, extra.ID)
	}
	if none, restrict := pickTag(t, h, sdk.ListProducts, "nobody-"+flag); !restrict || len(products(none, restrict)) != 0 {
		t.Error("a filter matching nothing left products in the list")
	}

	var custs []uuid.UUID
	for range 2 {
		c, err := h.D.Customer.AdminCreateCustomer(h.Ctx, customer.AdminCreateCustomerInput{
			ChannelID: ch, Email: "filter-" + uuid.NewString()[:8] + "@e2e.test", Password: "correct horse battery", FirstName: "Ana", LastName: "Filter",
		})
		if err != nil {
			t.Fatal(err)
		}
		custs = append(custs, c.ID)
	}
	taggedRecords.Store(flag, []uuid.UUID{custs[1]})
	only, restrict = pickTag(t, h, sdk.ListCustomers, flag)
	rows, total, err := h.D.Customer.ListPage(h.Ctx, customer.ListPageFilter{ChannelIDs: []uuid.UUID{ch}, Limit: 50, OnlyIDs: only, RestrictIDs: restrict})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != custs[1] {
		t.Errorf("filtered customers: %d of %d, %v - want only %s", len(rows), total, err, custs[1])
	}

	first, second := h.PlaceGuestCOD(t, f, 1), h.PlaceGuestCOD(t, f, 1)
	taggedRecords.Store(flag, []uuid.UUID{first.ID, uuid.New()})
	only, restrict = pickTag(t, h, sdk.ListOrders, flag)
	orders, count, err := h.D.Order.ListOrdersPage(h.Ctx, order.ListPageFilter{ChannelIDs: []uuid.UUID{ch}, Limit: 50, OnlyIDs: only, RestrictIDs: restrict})
	if err != nil || count != 1 || len(orders) != 1 || orders[0].ID != first.ID {
		t.Errorf("filtered orders: %d of %d, %v - want only %s, not %s", len(orders), count, err, first.ID, second.ID)
	}
}
