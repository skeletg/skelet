package admin

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// What this pins: a product type's template list reaches products on every store, so a catalog manager scoped to one
// store can't change it and a store-wide one can - and once set, the type, its products and the template all report
// it through the admin fields the dashboard reads

func TestOnlyAGlobalCatalogManagerSetsATypesTemplates(t *testing.T) {
	conn := testdb.Require(t)
	sfx := uuid.NewString()[:8]
	mine, typeID, productID, tplID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO channels (id, slug, name, currency_code) VALUES ($1, $2, 'mine', 'USD')`, []any{mine, "ott-" + sfx}},
		{`INSERT INTO catalog_product_types (id, slug, name) VALUES ($1, $2, 'Mugs')`, []any{typeID, "ott-" + sfx}},
		{`INSERT INTO catalog_products (id, product_type_id, sku, url, kind) VALUES ($1, $2, $3, $3, 'simple')`, []any{productID, typeID, "OTT-" + sfx}},
		{`INSERT INTO catalog_option_templates (id, name) VALUES ($1, $2)`, []any{tplID, "Care plan " + sfx}},
	} {
		if _, err := conn.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM catalog_option_template_product_types WHERE product_type_id = $1`,
			`DELETE FROM catalog_products WHERE product_type_id = $1`,
			`DELETE FROM catalog_product_types WHERE id = $1`,
		} {
			_, _ = conn.Exec(q, typeID)
		}
		_, _ = conn.Exec(`DELETE FROM catalog_option_templates WHERE id = $1`, tplID)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, mine)
	})
	queries, tx := db.New(conn), txrunner.New(conn)
	r := &Resolver{CatalogSvc: catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(queries, tx), Tx: tx})}
	m := &mutationResolver{r}
	channelAdmin := adminWithCachedGrants(auth.Grant{Slug: "catalog.manage", ChannelID: &mine})
	global := adminWithCachedGrants(auth.Grant{Slug: "catalog.manage"}, auth.Grant{Slug: "catalog.view"})

	if _, err := m.SetProductTypeOptionTemplates(channelAdmin, typeID.String(), []string{tplID.String()}); err == nil {
		t.Fatal("a store-scoped catalog manager set a type's templates")
	}
	var links int
	if err := conn.QueryRow(`SELECT count(*) FROM catalog_option_template_product_types WHERE product_type_id = $1`, typeID).Scan(&links); err != nil || links != 0 {
		t.Fatalf("the refused call left %d links (%v)", links, err)
	}
	if _, err := m.SetProductTypeOptionTemplates(global, typeID.String(), []string{"not-a-uuid"}); err == nil {
		t.Error("a malformed template id went through")
	}
	pt, err := m.SetProductTypeOptionTemplates(global, typeID.String(), []string{tplID.String()})
	if err != nil {
		t.Fatalf("a store-wide catalog manager was refused: %v", err)
	}
	if pt.ID != typeID.String() {
		t.Fatalf("returned type %q, want %s", pt.ID, typeID)
	}

	names := func(list []*model.OptionTemplate, err error) []string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(list))
		for i, x := range list {
			out[i] = x.Name
		}
		return out
	}
	want := []string{"Care plan " + sfx}
	if got := names((&productTypeResolver{r}).OptionTemplates(global, pt)); !slices.Equal(got, want) {
		t.Errorf("ProductType.optionTemplates = %v, want %v", got, want)
	}
	product := &model.Product{ID: productID.String()}
	if got := names((&productResolver{r}).TypeOptionTemplates(global, product)); !slices.Equal(got, want) {
		t.Errorf("Product.typeOptionTemplates = %v, want %v", got, want)
	}
	if got := names((&productResolver{r}).OptionTemplates(global, product)); len(got) != 0 {
		t.Errorf("Product.optionTemplates = %v, want none - the product picked nothing itself", got)
	}
	types, err := (&optionTemplateResolver{r}).ProductTypes(global, &model.OptionTemplate{ID: tplID.String()})
	if err != nil || len(types) != 1 || types[0].ID != typeID.String() || types[0].Name != "Mugs" {
		t.Errorf("OptionTemplate.productTypes = %+v %v, want Mugs", types, err)
	}
	listed, err := (&queryResolver{r}).OptionTemplates(global)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range listed {
		if x.ID == tplID.String() && (x.ProductTypeCount != 1 || x.ProductCount != 0) {
			t.Errorf("listed counts = %d types, %d products, want 1 and 0", x.ProductTypeCount, x.ProductCount)
		}
	}
}
