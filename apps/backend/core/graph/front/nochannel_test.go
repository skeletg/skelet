package front

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

func TestAStorelessRequestSeesNoCatalog(t *testing.T) {
	ctx := context.Background()
	r := &Resolver{}
	q := &queryResolver{r}
	id := uuid.New()

	if got := r.renderProductsByIDs(ctx, []uuid.UUID{id}); len(got) != 0 {
		t.Errorf("product rail rendered %d products without a store", len(got))
	}
	if got, err := q.RootCategories(ctx); err != nil || len(got) != 0 {
		t.Errorf("rootCategories = %d, %v", len(got), err)
	}
	if got, err := q.CategoryChildren(ctx, id.String()); err != nil || len(got) != 0 {
		t.Errorf("categoryChildren = %d, %v", len(got), err)
	}
	if _, err := q.Category(ctx, id.String()); !errors.Is(err, catalog.ErrCategoryNotFound) {
		t.Errorf("category = %v, want not found", err)
	}
	if got, err := q.Collections(ctx); err != nil || len(got) != 0 {
		t.Errorf("collections = %d, %v", len(got), err)
	}
	if _, err := q.Collection(ctx, id.String()); !errors.Is(err, catalog.ErrCollectionNotFound) {
		t.Errorf("collection = %v, want not found", err)
	}
	if p, err := r.fetchMembershipProduct(ctx, id); p != nil || err != nil {
		t.Errorf("membership product = %v, %v, want absent", p, err)
	}
	if kept, err := keepVisibleByChannel(ctx, r, []uuid.UUID{id}, func(u uuid.UUID) uuid.UUID { return u }); err != nil || len(kept) != 0 {
		t.Errorf("keepVisibleByChannel kept %d without a store", len(kept))
	}
	if _, err := r.renderBundleOptions(ctx, id); err == nil {
		t.Error("bundle options rendered without a store")
	}
}

func TestStockIsOnlyReadForTheRequestsStore(t *testing.T) {
	home := &channel.Channel{ID: uuid.New()}
	ctx := channel.WithChannel(context.Background(), home)
	r := &Resolver{}
	other := uuid.NewString()
	if _, err := r.resolveSalableChannel(ctx, &other); err == nil {
		t.Fatal("another channel's stock was readable from this store")
	}
	own := home.ID.String()
	if id, err := r.resolveSalableChannel(ctx, &own); err != nil || id != home.ID {
		t.Fatalf("own channel = %v, %v", id, err)
	}
	if id, _ := r.resolveSalableChannel(context.Background(), &own); id != uuid.Nil {
		t.Fatal("a storeless request named a channel's stock")
	}
}

func TestOnlyPublishedCollectionsReachTheStorefront(t *testing.T) {
	got := publishedCollections([]catalog.Collection{{Slug: "live", IsPublished: true}, {Slug: "draft"}})
	if len(got) != 1 || got[0].Slug != "live" {
		t.Fatalf("published = %+v, want only the live one", got)
	}
}
