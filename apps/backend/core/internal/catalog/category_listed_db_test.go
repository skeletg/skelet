package catalog

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The admin's store-scoped tree lists every category with a listing on the store, drafts and unpublished listings
// included - a category created from store scope starts as a draft and must stay findable there - while the
// storefront's published query keeps hiding all of them
func TestListedOnChannelIncludesDraftsTheStorefrontHides(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]

	var here, elsewhere uuid.UUID
	for i, id := range []*uuid.UUID{&here, &elsewhere} {
		if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'fx', 'USD') RETURNING id`, "cls-"+suffix+string(rune('a'+i))).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	var created []uuid.UUID
	category := func(name string, parent *uuid.UUID, published bool) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		slug := "cls-" + name + "-" + suffix
		if err := conn.QueryRow(`INSERT INTO catalog_categories (parent_id, slug, url_path, is_published) VALUES ($1, $2, $2, $3) RETURNING id`, parent, slug, published).Scan(&id); err != nil {
			t.Fatal(err)
		}
		created = append(created, id)
		return id
	}
	list := func(categoryID, channelID uuid.UUID, published bool, sortOverride *int32) {
		t.Helper()
		if _, err := conn.Exec(`INSERT INTO catalog_category_channel_listings (category_id, channel_id, is_published, sort_order_override) VALUES ($1, $2, $3, $4)`, categoryID, channelID, published, sortOverride); err != nil {
			t.Fatal(err)
		}
	}

	draft := category("draft", nil, false)
	list(draft, here, true, nil)
	unlistedHere := category("unpublished-listing", nil, true)
	list(unlistedHere, here, false, nil)
	child := category("child", &draft, false)
	override := int32(7)
	list(child, here, true, &override)
	otherStore := category("other-store", nil, true)
	list(otherStore, elsewhere, true, nil)
	category("nowhere", nil, true)

	t.Cleanup(func() {
		for _, id := range slices.Backward(created) {
			_, _ = conn.Exec(`DELETE FROM catalog_categories WHERE id = $1`, id)
		}
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = ANY($1)`, []uuid.UUID{here, elsewhere})
	})
	repo := &postgresRepository{queries: db.New(conn)}

	got, err := repo.ListCategoriesListedOnChannel(ctx, here)
	if err != nil {
		t.Fatal(err)
	}
	if ids := categoryIDs(got); len(ids) != 3 || !slices.Contains(ids, draft) || !slices.Contains(ids, unlistedHere) || !slices.Contains(ids, child) {
		t.Fatalf("listed on the store = %v, want the draft, the unpublished listing and the child (%v %v %v)", ids, draft, unlistedHere, child)
	}
	for _, c := range got {
		if c.ID == child && c.Position != override {
			t.Fatalf("child position = %d, want the store's sort override %d", c.Position, override)
		}
	}

	children, err := repo.ListCategoryChildrenListedOnChannel(ctx, here, draft)
	if err != nil {
		t.Fatal(err)
	}
	if ids := categoryIDs(children); len(ids) != 1 || ids[0] != child {
		t.Fatalf("children listed on the store = %v, want only %v", ids, child)
	}
	if none, err := repo.ListCategoryChildrenListedOnChannel(ctx, elsewhere, draft); err != nil || len(none) != 0 {
		t.Fatalf("children on a store the child isn't listed on = %v, %v, want none", categoryIDs(none), err)
	}

	published, err := repo.ListPublishedCategoriesForChannel(ctx, here)
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 0 {
		t.Fatalf("the storefront query must still hide drafts and unpublished listings, got %v", categoryIDs(published))
	}
}
