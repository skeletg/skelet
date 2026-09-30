package blog

import (
	"context"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// The opt-in is the only thing keeping an author's address off the storefront, so an unrelated edit must never flip it
func TestAuthorEmailPublicIsSavedAndKeptAcrossEdits(t *testing.T) {
	conn := testdb.Require(t)
	repo := NewRepository(db.New(conn), txrunner.New(conn))
	ctx := context.Background()
	addr := "jane@example.com"

	a, err := repo.CreateAuthor(ctx, CreateAuthorInput{Slug: "au-" + uuid.NewString()[:8], DisplayName: "Jane", Email: &addr})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM blog_authors WHERE id = $1`, a.ID) })
	if a.EmailPublic {
		t.Fatal("a new author's email started public")
	}

	on := true
	if a, err = repo.UpdateAuthor(ctx, UpdateAuthorInput{ID: a.ID, EmailPublic: &on}); err != nil || !a.EmailPublic {
		t.Fatalf("turning it on = (%v, %v)", a.EmailPublic, err)
	}
	name := "Jane Doe"
	if a, err = repo.UpdateAuthor(ctx, UpdateAuthorInput{ID: a.ID, DisplayName: &name}); err != nil || !a.EmailPublic {
		t.Errorf("an edit that didn't mention it turned it off: (%v, %v)", a.EmailPublic, err)
	}
	if got, err := repo.GetAuthorByID(ctx, a.ID); err != nil || !got.EmailPublic {
		t.Errorf("stored value = (%v, %v), want on", got.EmailPublic, err)
	}
}
