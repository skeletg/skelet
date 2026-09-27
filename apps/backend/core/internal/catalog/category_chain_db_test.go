package catalog

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestAncestorsComeBackRootFirstFromOneQuery(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	ids := make([]uuid.UUID, 3) // root, mid, leaf
	var parent *uuid.UUID
	for i, name := range []string{"root", "mid", "leaf"} {
		slug := "chain-" + name + "-" + suffix
		if err := conn.QueryRow(`INSERT INTO catalog_categories (parent_id, slug, url_path) VALUES ($1, $2, $2) RETURNING id`, parent, slug).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
		parent = &ids[i]
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`UPDATE catalog_categories SET parent_id = NULL WHERE id = ANY($1)`, ids)
		for _, id := range slices.Backward(ids) {
			_, _ = conn.Exec(`DELETE FROM catalog_categories WHERE id = $1`, id)
		}
	})
	repo := &postgresRepository{queries: db.New(conn)}

	got, err := repo.GetCategoryAncestors(ctx, ids[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != ids[0] || got[1].ID != ids[1] {
		t.Fatalf("want [root mid], got %v", categoryIDs(got))
	}
	if got, err := repo.GetCategoryAncestors(ctx, ids[0]); err != nil || len(got) != 0 {
		t.Fatalf("a root has no ancestors, got %v, %v", categoryIDs(got), err)
	}
	if _, err := repo.GetCategoryAncestors(ctx, uuid.New()); !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("an unknown id keeps reporting not found, got %v", err)
	}

	if _, err := conn.Exec(`UPDATE catalog_categories SET parent_id = $1 WHERE id = $2`, ids[2], ids[0]); err != nil {
		t.Skipf("schema refuses a parent cycle, nothing left to guard: %v", err)
	}
	if _, err := repo.GetCategoryAncestors(ctx, ids[2]); err != nil {
		t.Fatalf("a corrupted cycle must terminate, got %v", err)
	}
}

func categoryIDs(cs []Category) []uuid.UUID {
	out := make([]uuid.UUID, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}
