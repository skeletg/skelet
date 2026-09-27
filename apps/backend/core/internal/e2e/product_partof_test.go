package e2e

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// Scenario: a variant's partOf.configurable answers its parent, its visible siblings and its values on the parent's axis, private ones included
func TestVariantMembershipListsVisibleSiblingsAndItsAxisValues(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 1500)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	suffix := uuid.NewString()[:8]
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := h.D.DB.ExecContext(h.Ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}

	var attrID uuid.UUID
	if err := h.D.DB.QueryRowContext(h.Ctx, `INSERT INTO catalog_attributes (slug, name, input_type, is_public) VALUES ($1, 'Size', 'dropdown', false) RETURNING id`, "size-"+suffix).Scan(&attrID); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO catalog_product_type_attributes (product_type_id, attribute_id) VALUES ($1, $2)`, f.Product.ProductTypeID, attrID)

	var parentID uuid.UUID
	if err := h.D.DB.QueryRowContext(h.Ctx, `INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'configurable') RETURNING id`,
		f.Product.ProductTypeID, "partof-parent-"+suffix).Scan(&parentID); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, true)`, parentID, f.Channel.ID)
	exec(`INSERT INTO catalog_product_configurable_options (product_id, attribute_id) VALUES ($1, $2)`, parentID, attrID)

	var variants, values []uuid.UUID
	for i, size := range []string{"s", "m", "l"} {
		v := h.AddProduct(t, f, 1000, i < 2, 5)
		var valueID uuid.UUID
		if err := h.D.DB.QueryRowContext(h.Ctx, `INSERT INTO catalog_attribute_values (attribute_id, slug, name) VALUES ($1, $2, $2) RETURNING id`, attrID, size+"-"+suffix).Scan(&valueID); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO catalog_product_children (parent_id, child_id, sort_order) VALUES ($1, $2, $3)`, parentID, v.ID, i)
		exec(`INSERT INTO catalog_product_attribute_values (product_id, attribute_id, attribute_value_id) VALUES ($1, $2, $3)`, v.ID, attrID, valueID)
		variants = append(variants, v.ID)
		values = append(values, valueID)
	}

	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`{ product(id: %q) { partOf { configurable {
		parent { id } siblings { id } sortOrder optionValues { attribute { id } selectedValues { id } } } } } }`, variants[0]))
	if len(errs) > 0 {
		t.Fatalf("partOf: %v", errs)
	}
	cfg, ok := data["product"].(map[string]any)["partOf"].(map[string]any)["configurable"].(map[string]any)
	if !ok {
		t.Fatalf("variant has no configurable membership: %v", data)
	}
	if got := cfg["parent"].(map[string]any)["id"]; got != parentID.String() {
		t.Fatalf("parent = %v, want %s", got, parentID)
	}
	var siblings []string
	for _, s := range cfg["siblings"].([]any) {
		siblings = append(siblings, s.(map[string]any)["id"].(string))
	}
	if len(siblings) != 1 || siblings[0] != variants[1].String() {
		t.Fatalf("siblings = %v, want only the other visible variant %s", siblings, variants[1])
	}
	found := false
	for _, ov := range cfg["optionValues"].([]any) {
		m := ov.(map[string]any)
		if m["attribute"].(map[string]any)["id"] != attrID.String() {
			continue
		}
		for _, sv := range m["selectedValues"].([]any) {
			found = found || sv.(map[string]any)["id"] == values[0].String()
		}
	}
	if !found {
		t.Fatalf("optionValues must carry the variant's own value on the private axis attribute, got %v", cfg["optionValues"])
	}
}
