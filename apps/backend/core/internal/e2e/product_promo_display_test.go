package e2e

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// Scenario: a pill scoped to a category badges that category's products wherever they are read
func TestCategoryScopedPillBadgesEveryProductRead(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 1500)
	other := h.AddProduct(t, f, 900, true, 5)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	suffix := uuid.NewString()[:8]

	var catID uuid.UUID
	if err := h.D.DB.QueryRowContext(h.Ctx, `INSERT INTO catalog_categories (slug, url_path, is_published) VALUES ($1, $1, true) RETURNING id`, "promo-cat-"+suffix).Scan(&catID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.DB.ExecContext(h.Ctx, `INSERT INTO catalog_category_channel_listings (category_id, channel_id, is_published) VALUES ($1, $2, true)`, catID, f.Channel.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.DB.ExecContext(h.Ctx, `INSERT INTO catalog_product_categories (product_id, category_id, position, is_primary) VALUES ($1, $2, 0, false)`, f.Product.ID, catID); err != nil {
		t.Fatal(err)
	}
	slug := "e2e-pill-" + suffix
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: slug, Name: "E2E category pill", ChannelID: &f.Channel.ID, IsActive: true, PillText: "Sale",
		},
		Conditions: []promotion.Condition{{Type: "core.product_in_category",
			Params: json.RawMessage(fmt.Sprintf(`{"category_ids":[%q]}`, catID))}},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal", Params: json.RawMessage(`{"cents":100}`)}},
	})

	badged := func(node any) bool {
		for _, d := range node.(map[string]any)["promotionalDisplays"].([]any) {
			if d.(map[string]any)["ruleSlug"] == slug {
				return true
			}
		}
		return false
	}
	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`{ product(id: %q) { promotionalDisplays { ruleSlug } } }`, f.Product.ID))
	if len(errs) > 0 {
		t.Fatalf("product: %v", errs)
	}
	if !badged(data["product"]) {
		t.Fatal("the product page must badge a product in the pill's category")
	}
	data, errs = h.gqlPost(t, ts, chID, fmt.Sprintf(`{ productsByIds(ids: [%q, %q]) { id promotionalDisplays { ruleSlug } } }`, f.Product.ID, other.ID))
	if len(errs) > 0 {
		t.Fatalf("productsByIds: %v", errs)
	}
	for _, n := range data["productsByIds"].([]any) {
		id := n.(map[string]any)["id"]
		if want := id == f.Product.ID.String(); badged(n) != want {
			t.Fatalf("card %v badged = %v, want %v", id, !want, want)
		}
	}
}
