package front

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// A published child under an unpublished parent has a broken breadcrumb, so the storefront treats it as absent - and a visible one carries its parent
func TestTheStrictAncestorRuleHidesOrphanedChildren(t *testing.T) {
	conn := testdb.Require(t)
	ch := uuid.New()
	sfx := ch.String()[:8]
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code) VALUES ($1, $2, 'vis', 'USD')`, ch, "vis-"+sfx); err != nil {
		t.Fatal(err)
	}
	root, hidden, orphan, child := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, c := range []struct {
		id, parent uuid.UUID
		name       string
		listed     bool
	}{
		{root, uuid.Nil, "root", true},
		{hidden, root, "hidden", false},
		{orphan, hidden, "orphan", true},
		{child, root, "child", true},
	} {
		parent := uuid.NullUUID{UUID: c.parent, Valid: c.parent != uuid.Nil}
		slug := "vis-" + c.name + "-" + sfx
		if _, err := conn.Exec(`INSERT INTO catalog_categories (id, parent_id, slug, url_path, is_published) VALUES ($1, $2, $3, $3, TRUE)`, c.id, parent, slug); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`INSERT INTO catalog_category_channel_listings (category_id, channel_id, is_published) VALUES ($1, $2, $3)`, c.id, ch, c.listed); err != nil {
			t.Fatal(err)
		}
	}
	tx := txrunner.New(conn)
	r := &queryResolver{&Resolver{CatalogSvc: catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(db.New(conn), tx), Tx: tx})}}
	ctx := channel.WithChannel(t.Context(), &channel.Channel{ID: ch})

	if _, err := r.Category(ctx, orphan.String()); !errors.Is(err, catalog.ErrCategoryNotFound) {
		t.Errorf("category(orphan) err = %v, want not found - its parent is unpublished here", err)
	}
	kids, err := r.CategoryChildren(ctx, hidden.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 0 {
		t.Errorf("categoryChildren(unpublished parent) = %d rows, want none", len(kids))
	}

	got, err := r.Category(ctx, child.String())
	if err != nil {
		t.Fatalf("category(child): %v", err)
	}
	if got.Parent == nil || got.Parent.ID != root.String() {
		t.Errorf("category(child).parent = %+v, want the root", got.Parent)
	}
	kids, err = r.CategoryChildren(ctx, root.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 1 || kids[0].ID != child.String() || kids[0].Parent == nil || kids[0].Parent.ID != root.String() {
		t.Errorf("categoryChildren(root) = %+v, want only the visible child, parented", kids)
	}
}
