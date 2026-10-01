package admin

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// A rename or move rewrites paths on every channel the subtree is listed on, so a channel grant covering only the category's own listing isn't enough
func TestOnlyAGlobalAdminRenamesOrMovesACategory(t *testing.T) {
	conn := testdb.Require(t)
	mine := uuid.New()
	sfx := mine.String()[:8]
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code) VALUES ($1, $2, 'mine', 'USD')`, mine, "mv-"+sfx); err != nil {
		t.Fatal(err)
	}
	catID, otherID := uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO catalog_categories (id, slug, url_path) VALUES ($1, $2, $2)`, []any{catID, "mv-cat-" + sfx}},
		{`INSERT INTO catalog_categories (id, slug, url_path) VALUES ($1, $2, $2)`, []any{otherID, "mv-other-" + sfx}},
		{`INSERT INTO catalog_category_channel_listings (category_id, channel_id) VALUES ($1, $2)`, []any{catID, mine}},
	} {
		if _, err := conn.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	queries, tx := db.New(conn), txrunner.New(conn)
	r := &mutationResolver{&Resolver{CatalogSvc: catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(queries, tx), Tx: tx})}}
	channelAdmin := adminWithCachedGrants(auth.Grant{Slug: "catalog.manage", ChannelID: &mine})
	global := adminWithCachedGrants(auth.Grant{Slug: "catalog.manage"})

	slug, same := "mv-renamed-"+sfx, "mv-cat-"+sfx
	parent := otherID.String()
	pos := int32(3)
	if _, err := r.UpdateCategory(channelAdmin, model.UpdateCategoryInput{ID: catID.String(), Slug: &slug}); err == nil {
		t.Error("a channel admin renamed a category")
	}
	if _, err := r.UpdateCategory(channelAdmin, model.UpdateCategoryInput{ID: catID.String(), ParentID: &parent}); err == nil {
		t.Error("a channel admin moved a category")
	}
	if _, err := r.UpdateCategory(channelAdmin, model.UpdateCategoryInput{ID: catID.String(), Slug: &same, Position: &pos}); err != nil {
		t.Errorf("a channel admin resaving the same slug with a new position was refused: %v", err)
	}
	if _, err := r.UpdateCategory(global, model.UpdateCategoryInput{ID: catID.String(), Slug: &slug}); err != nil {
		t.Errorf("a global admin's rename was refused: %v", err)
	}
}

// A tree's productCount fields share one grouped COUNT, and every node still gets its own number
func TestCategoryProductCountsComeFromTheBatch(t *testing.T) {
	conn := testdb.Require(t)
	sfx := uuid.NewString()[:8]
	full, empty, typeID := uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO catalog_categories (id, slug, url_path) VALUES ($1, $2, $2), ($3, $4, $4)`, []any{full, "cnt-full-" + sfx, empty, "cnt-empty-" + sfx}},
		{`INSERT INTO catalog_product_types (id, slug, name) VALUES ($1, $2, 'cnt')`, []any{typeID, "cnt-" + sfx}},
		{`WITH p AS (INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple'), ($1, $3, $3, 'simple') RETURNING id)
		  INSERT INTO catalog_product_categories (product_id, category_id) SELECT id, $4 FROM p`, []any{typeID, "CNT-A-" + sfx, "CNT-B-" + sfx, full}},
	} {
		if _, err := conn.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	queries, tx := db.New(conn), txrunner.New(conn)
	r := &Resolver{CatalogSvc: catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(queries, tx), Tx: tx})}
	ctx := withRenderCache(t.Context())
	r.noteCategoryList(ctx, []uuid.UUID{full, empty})
	if got, err := r.getCategoryProductCount(ctx, full); err != nil || got != 2 {
		t.Errorf("count(full) = %d, %v, want 2", got, err)
	}
	if c := getRenderCache(ctx); len(c.categoryProductCounts) != 2 {
		t.Errorf("the first count loaded %d categories, want both noted ones", len(c.categoryProductCounts))
	}
	if got, err := r.getCategoryProductCount(ctx, empty); err != nil || got != 0 {
		t.Errorf("count(empty) = %d, %v, want 0", got, err)
	}
}
