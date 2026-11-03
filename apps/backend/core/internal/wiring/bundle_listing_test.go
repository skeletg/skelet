package wiring

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

func TestABundleIsSoldOnlyWhereItsListingIsPublished(t *testing.T) {
	home, draft, other := uuid.New(), uuid.New(), uuid.New()
	listings := []catalog.ProductChannelListing{{ChannelID: home, IsPublished: true}, {ChannelID: draft}}
	if !publishedOn(listings, home) {
		t.Fatal("a published listing was refused")
	}
	if publishedOn(listings, draft) || publishedOn(listings, other) {
		t.Fatal("a bundle sold where it is unpublished or unlisted")
	}
}

type missingSKURepo struct {
	catalog.ProductRepository
}

func (missingSKURepo) GetProductBySKU(context.Context, string) (catalog.Product, error) {
	return catalog.Product{}, catalog.ErrProductNotFound
}

// Checkout refuses only on ErrProductNotPublished, so a deleted bundle has to read as that - while a real lookup failure must stay an outage, not a refusal
func TestADeletedBundleReadsAsUnpublished(t *testing.T) {
	gone := CartCatalogAdapter{svc: catalog.NewService(catalog.Deps{Repo: catalog.ReposOf(missingSKURepo{})})}
	if _, err := gone.ResolveBundleBySKU(context.Background(), uuid.New(), "set"); !errors.Is(err, cart.ErrProductNotPublished) {
		t.Fatalf("deleted bundle err = %v, want ErrProductNotPublished", err)
	}
	down := CartCatalogAdapter{svc: catalog.NewService(catalog.Deps{Repo: catalog.ReposOf(failingSKURepo{})})}
	if _, err := down.ResolveBundleBySKU(context.Background(), uuid.New(), "set"); errors.Is(err, cart.ErrProductNotPublished) || !errors.Is(err, errCatalogDown) {
		t.Fatalf("catalog outage err = %v, want the lookup failure", err)
	}
}
