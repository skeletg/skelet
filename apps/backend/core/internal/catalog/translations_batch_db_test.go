package catalog

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The admin Category.content, Product.content and Collection.content fields read every locale's row through these batch loads.
// List paths used to answer [] there, and the dashboard's cache let that empty list replace the rows the
// detail query had fetched - the next save wrote the blanks over the stored translations

// Two locales of its own, so "every locale" is more than the one the dev data happens to hold
func seedTranslationLocales(t *testing.T, conn *sql.DB) (string, string) {
	t.Helper()
	suffix := uuid.NewString()[:6]
	a, b := "ta"+suffix, "tb"+suffix
	for _, code := range []string{a, b} {
		if _, err := conn.Exec(`INSERT INTO locales (code, name) VALUES ($1, $1)`, code); err != nil {
			t.Fatalf("seed locale %s: %v", code, err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM locales WHERE code = $1`, code) }) // after the rows below cascade away
	}
	return a, b
}

func TestListCategoryContentForCategoriesReturnsEveryLocale(t *testing.T) {
	conn := testdb.Require(t)
	a, b := seedTranslationLocales(t, conn)
	seed := func(slug string, locales ...string) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO catalog_categories (slug, url_path) VALUES ($1, $1) RETURNING id`, slug).Scan(&id); err != nil {
			t.Fatalf("seed category: %v", err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM catalog_categories WHERE id = $1`, id) })
		for _, loc := range locales {
			if _, err := conn.Exec(`INSERT INTO catalog_category_content (category_id, locale, name) VALUES ($1, $2, $3)`, id, loc, slug+" "+loc); err != nil {
				t.Fatalf("seed category content: %v", err)
			}
		}
		return id
	}
	suffix := uuid.NewString()[:8]
	both := seed("tr-both-"+suffix, a, b)
	one := seed("tr-one-"+suffix, b)
	none := seed("tr-none-" + suffix)

	repo := NewRepository(db.New(conn), nil)
	got, err := repo.ListCategoryContentForCategories(context.Background(), []uuid.UUID{both, one, none})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if locales := contentLocales(got[both], func(c CategoryContent) string { return c.Locale }); !slices.Equal(locales, []string{a, b}) {
		t.Errorf("category with two translations came back with %v, want [%s %s]", locales, a, b)
	}
	if locales := contentLocales(got[one], func(c CategoryContent) string { return c.Locale }); !slices.Equal(locales, []string{b}) {
		t.Errorf("category with one translation came back with %v, want [%s]", locales, b)
	}
	if rows, ok := got[none]; ok {
		t.Errorf("category with no translations came back with %v, want no entry", rows)
	}

	if empty, err := repo.ListCategoryContentForCategories(context.Background(), nil); err != nil || len(empty) != 0 {
		t.Errorf("no ids: got %v, %v - want an empty map and no query error", empty, err)
	}
}

func TestListProductContentForProductsReturnsEveryLocale(t *testing.T) {
	conn := testdb.Require(t)
	a, b := seedTranslationLocales(t, conn)
	suffix := uuid.NewString()[:8]
	var typeID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, $1) RETURNING id`, "tr-type-"+suffix).Scan(&typeID); err != nil {
		t.Fatalf("seed product type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID) }) // after the products below
	seed := func(sku string, locales ...string) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, sku).Scan(&id); err != nil {
			t.Fatalf("seed product: %v", err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, id) })
		for _, loc := range locales {
			if _, err := conn.Exec(`INSERT INTO catalog_product_content (product_id, locale, name) VALUES ($1, $2, $3)`, id, loc, sku+" "+loc); err != nil {
				t.Fatalf("seed product content: %v", err)
			}
		}
		return id
	}
	both := seed("tr-both-"+suffix, a, b)
	none := seed("tr-none-" + suffix)

	repo := NewRepository(db.New(conn), nil)
	got, err := repo.ListProductContentForProducts(context.Background(), []uuid.UUID{both, none})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if locales := contentLocales(got[both], func(c ProductContent) string { return c.Locale }); !slices.Equal(locales, []string{a, b}) {
		t.Errorf("product with two translations came back with %v, want [%s %s]", locales, a, b)
	}
	if rows, ok := got[none]; ok {
		t.Errorf("product with no translations came back with %v, want no entry", rows)
	}
}

func TestListCollectionContentForCollectionsReturnsEveryLocale(t *testing.T) {
	conn := testdb.Require(t)
	a, b := seedTranslationLocales(t, conn)
	suffix := uuid.NewString()[:8]
	seed := func(slug string, locales ...string) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO catalog_collections (slug) VALUES ($1) RETURNING id`, slug).Scan(&id); err != nil {
			t.Fatalf("seed collection: %v", err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM catalog_collections WHERE id = $1`, id) })
		for _, loc := range locales {
			if _, err := conn.Exec(`INSERT INTO catalog_collection_content (collection_id, locale, name) VALUES ($1, $2, $3)`, id, loc, slug+" "+loc); err != nil {
				t.Fatalf("seed collection content: %v", err)
			}
		}
		return id
	}
	both := seed("tr-both-"+suffix, a, b)
	none := seed("tr-none-" + suffix)

	repo := NewRepository(db.New(conn), nil)
	got, err := repo.ListCollectionContentForCollections(context.Background(), []uuid.UUID{both, none})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if locales := contentLocales(got[both], func(c CollectionContent) string { return c.Locale }); !slices.Equal(locales, []string{a, b}) {
		t.Errorf("collection with two translations came back with %v, want [%s %s]", locales, a, b)
	}
	if rows, ok := got[none]; ok {
		t.Errorf("collection with no translations came back with %v, want no entry", rows)
	}
}

func contentLocales[T any](rows []T, locale func(T) string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = locale(r)
	}
	return out
}
