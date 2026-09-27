package e2e

import (
	"fmt"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

const narrowProductFields = `id name sku kind url __typename reviewSummary
	reviews(limit: 5) { id } reviewAggregates { count overallAverage }
	reviewQuestions { id } customerCanReview { canReview reason }
	linkedProducts(kind: RELATED) { id } alsoBought(limit: 4) { id }`

func productQuery(id uuid.UUID, extra string) string {
	return fmt.Sprintf(`{ product(id: %q) { %s %s } }`, id, narrowProductFields, extra)
}

// Scenario: a narrow product(id) read answers and hides exactly what a wider one does for the same fields
func TestNarrowProductReadMatchesAWiderOne(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 1500)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()

	light, errs := h.gqlPost(t, ts, chID, productQuery(f.Product.ID, ""))
	if len(errs) > 0 {
		t.Fatalf("light read: %v", errs)
	}
	full, errs := h.gqlPost(t, ts, chID, productQuery(f.Product.ID, "description"))
	if len(errs) > 0 {
		t.Fatalf("full read: %v", errs)
	}
	fullProduct := full["product"].(map[string]any)
	delete(fullProduct, "description")
	if !reflect.DeepEqual(light["product"], fullProduct) {
		t.Fatalf("narrow read diverged from the wider one\nlight: %v\nfull:  %v", light["product"], fullProduct)
	}
	if light["product"].(map[string]any)["name"] == nil {
		t.Fatal("the narrow read must resolve the localized name")
	}

	other := h.NewStore(t, 5, 1500)
	for _, extra := range []string{"", "description"} {
		if _, errs := h.gqlPost(t, ts, other.Channel.ID.String(), productQuery(f.Product.ID, extra)); len(errs) == 0 {
			t.Fatalf("a product unpublished on the channel must stay not found (extra=%q)", extra)
		}
	}

	suffix := uuid.NewString()[:8]
	var parentID, childID uuid.UUID
	if err := h.D.DB.QueryRowContext(h.Ctx, `INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'configurable') RETURNING id`,
		f.Product.ProductTypeID, "LIGHT-CFG-"+suffix).Scan(&parentID); err != nil {
		t.Fatal(err)
	}
	if err := h.D.DB.QueryRowContext(h.Ctx, `INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`,
		f.Product.ProductTypeID, "light-var-"+suffix).Scan(&childID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.DB.ExecContext(h.Ctx, `INSERT INTO catalog_product_children (parent_id, child_id) VALUES ($1, $2)`, parentID, childID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.DB.ExecContext(h.Ctx, `INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, true)`, parentID, f.Channel.ID); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"", "description"} {
		if _, errs := h.gqlPost(t, ts, chID, productQuery(parentID, extra)); len(errs) == 0 {
			t.Fatalf("a configurable with every variant hidden must be not found (extra=%q)", extra)
		}
	}
	if _, err := h.D.DB.ExecContext(h.Ctx, `INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, true)`, childID, f.Channel.ID); err != nil {
		t.Fatal(err)
	}
	lightCfg, errs := h.gqlPost(t, ts, chID, productQuery(parentID, ""))
	if len(errs) > 0 {
		t.Fatalf("a configurable with a visible variant must resolve: %v", errs)
	}
	fullCfg, errs := h.gqlPost(t, ts, chID, productQuery(parentID, "description"))
	if len(errs) > 0 {
		t.Fatalf("full configurable read: %v", errs)
	}
	fp := fullCfg["product"].(map[string]any)
	delete(fp, "description")
	if !reflect.DeepEqual(lightCfg["product"], fp) {
		t.Fatalf("configurable narrow read diverged\nlight: %v\nfull:  %v", lightCfg["product"], fp)
	}
}
