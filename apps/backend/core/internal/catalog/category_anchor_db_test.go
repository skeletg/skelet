package catalog

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// An anchor's page reaches every category beneath it that the channel shows - published globally and listed published
// there. One the channel hides, either way, hides its branch, and a category that isn't an anchor lists only itself.
// A new category starts as an anchor
func TestAnAnchorCategoryListsTheBranchItsChannelShows(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := &postgresRepository{queries: db.New(conn)}
	suffix := uuid.NewString()[:8]
	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'anc', 'USD') RETURNING id`, "anc-"+suffix).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch) })
	category := func(name string, parent *uuid.UUID, published, listed bool) uuid.UUID {
		t.Helper()
		c, err := repo.CreateCategory(ctx, CreateCategoryInput{ParentID: parent, Slug: "anc-" + name + "-" + suffix, IsPublished: published})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM catalog_categories WHERE id = $1`, c.ID) })
		if !c.IsAnchor {
			t.Fatalf("%s was created without being an anchor", name)
		}
		if _, err := repo.UpsertCategoryChannelListing(ctx, SetCategoryChannelListingInput{CategoryID: c.ID, ChannelID: ch, IsPublished: listed}); err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	root := category("root", nil, true, true)
	shirts := category("shirts", &root, true, true)
	linen := category("linen", &shirts, true, true)
	hidden := category("hidden", &root, false, true)
	category("under-hidden", &hidden, true, true)
	workwear := category("workwear", &root, true, false)
	category("under-workwear", &workwear, true, true)

	listed := func() []uuid.UUID {
		t.Helper()
		ids, err := repo.ListCategoryListingIDs(ctx, ch, root)
		if err != nil {
			t.Fatal(err)
		}
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		return ids
	}
	want := []uuid.UUID{root, shirts, linen}
	slices.SortFunc(want, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	if got := listed(); !slices.Equal(got, want) {
		t.Fatalf("an anchor lists %v, want itself and the branch the channel shows %v", got, want)
	}

	off := false
	if _, err := repo.UpdateCategory(ctx, UpdateCategoryInput{ID: root, IsAnchor: &off}); err != nil {
		t.Fatal(err)
	}
	if got := listed(); !slices.Equal(got, []uuid.UUID{root}) {
		t.Fatalf("a category that isn't an anchor lists %v, want only itself", got)
	}
}
