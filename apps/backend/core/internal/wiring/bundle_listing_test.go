package wiring

import (
	"testing"

	"github.com/google/uuid"

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
