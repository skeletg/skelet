package e2e

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// Scenario: a category unpublished on the channel stays out of a product's categories and breadcrumb on every read path
func TestUnpublishedCategoriesStayHiddenOnEveryProductPath(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 1500)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()

	slug := "hidden-" + uuid.NewString()[:8]
	var catID uuid.UUID
	if err := h.D.DB.QueryRowContext(h.Ctx, `INSERT INTO catalog_categories (slug, url_path, is_published) VALUES ($1, $1, true) RETURNING id`, slug).Scan(&catID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.DB.ExecContext(h.Ctx, `INSERT INTO catalog_product_categories (product_id, category_id, position, is_primary) VALUES ($1, $2, 0, true)`, f.Product.ID, catID); err != nil {
		t.Fatal(err)
	}

	reads := map[string]string{
		"product":       fmt.Sprintf(`{ product(id: %q) { categories { id } primaryCategory { id } } }`, f.Product.ID),
		"productsByIds": fmt.Sprintf(`{ productsByIds(ids: [%q]) { categories { id } primaryCategory { id } } }`, f.Product.ID),
	}
	categoryIDs := func(data map[string]any, root string) (cats []string, primary string) {
		node := data[root]
		if list, ok := node.([]any); ok {
			if len(list) != 1 {
				t.Fatalf("%s returned %d products, want 1", root, len(list))
			}
			node = list[0]
		}
		p := node.(map[string]any)
		for _, c := range p["categories"].([]any) {
			cats = append(cats, c.(map[string]any)["id"].(string))
		}
		if pc, ok := p["primaryCategory"].(map[string]any); ok {
			primary = pc["id"].(string)
		}
		return cats, primary
	}

	for root, q := range reads {
		data, errs := h.gqlPost(t, ts, chID, q)
		if len(errs) > 0 {
			t.Fatalf("%s: %v", root, errs)
		}
		cats, primary := categoryIDs(data, root)
		for _, c := range cats {
			if c == catID.String() {
				t.Fatalf("%s leaked a category unpublished on the channel", root)
			}
		}
		if primary == catID.String() {
			t.Fatalf("%s leaked an unpublished primary category", root)
		}
	}

	if _, err := h.D.DB.ExecContext(h.Ctx, `INSERT INTO catalog_category_channel_listings (category_id, channel_id, is_published) VALUES ($1, $2, true)`, catID, f.Channel.ID); err != nil {
		t.Fatal(err)
	}
	for root, q := range reads {
		data, errs := h.gqlPost(t, ts, chID, q)
		if len(errs) > 0 {
			t.Fatalf("%s after publishing: %v", root, errs)
		}
		cats, primary := categoryIDs(data, root)
		found := false
		for _, c := range cats {
			found = found || c == catID.String()
		}
		if !found || primary != catID.String() {
			t.Fatalf("%s must return the category once it is published on the channel, got categories %v primary %q", root, cats, primary)
		}
	}
}
