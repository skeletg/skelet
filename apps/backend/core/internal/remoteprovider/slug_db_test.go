package remoteprovider

import (
	"context"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestAProviderSlugIsTheChannelsOwnBeforeTheGlobalOne(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn), nil)
	a, b := uuid.New(), uuid.New()
	for _, ch := range []uuid.UUID{a, b} {
		if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'Slug','USD',true)`, ch, "slug-"+ch.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	slug := "rates-" + uuid.NewString()[:8]
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM remote_providers WHERE slug = $1`, slug)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = ANY($1)`, "{"+a.String()+","+b.String()+"}")
	})
	mk := func(ch *uuid.UUID, name string) Provider {
		p, err := repo.Create(ctx, CreateInput{Kind: KindPricing, Slug: slug, Name: name, TargetURL: "https://p.example.test", Config: []byte("{}"), TimeoutMs: 1000, FailurePolicy: FailOpen, ChannelID: ch}, "secret")
		if err != nil {
			t.Fatalf("create %s: %v - two channels must be able to use one slug", name, err)
		}
		return p
	}
	global := mk(nil, "global")
	own := mk(&a, "a")
	mk(&b, "b")

	if got, err := repo.GetBySlug(ctx, KindPricing, slug, &a); err != nil || got.ID != own.ID {
		t.Fatalf("channel a resolved %v, %v, want its own provider", got.Name, err)
	}
	other := uuid.New()
	if got, err := repo.GetBySlug(ctx, KindPricing, slug, &other); err != nil || got.ID != global.ID {
		t.Fatalf("a channel without its own resolved %v, %v, want the global one", got.Name, err)
	}
	if _, err := repo.Create(ctx, CreateInput{Kind: KindPricing, Slug: slug, Name: "dup", TargetURL: "https://p.example.test", Config: []byte("{}"), TimeoutMs: 1000, FailurePolicy: FailOpen, ChannelID: &a}, "secret"); err == nil {
		t.Fatal("a second provider under one channel's slug was accepted")
	}
}
