package wiring

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

var errCatalogDown = errors.New("catalog read failed")

type failingSKURepo struct {
	catalog.ProductRepository
}

func (failingSKURepo) GetProductBySKU(context.Context, string) (catalog.Product, error) {
	return catalog.Product{}, errCatalogDown
}

// Only not-published / not-purchasable make a linked value unpickable; anything else is a real failure and must not quietly price the value as unavailable
func TestLinkedValueSourcePricingErrorSurfaces(t *testing.T) {
	a := CartCatalogAdapter{svc: catalog.NewService(catalog.Deps{Repo: catalog.ReposOf(failingSKURepo{})})}
	src := uuid.New()
	opts := []catalog.ProductOption{{
		ID:        uuid.New(),
		InputType: catalog.OptionInputDropdown,
		Values:    []catalog.ProductOptionValue{{ID: uuid.New(), SourceProductID: &src, SourceProductSKU: "SRC-1", Quantity: 1}},
	}}
	var res cart.ProductOptionsResolution
	err := a.appendCartOptions(context.Background(), channel.Channel{ID: uuid.New(), CurrencyCode: "USD"}, &res, opts)
	if !errors.Is(err, errCatalogDown) {
		t.Fatalf("appendCartOptions err = %v, want the source lookup failure", err)
	}
	if len(res.Options) != 0 {
		t.Fatalf("options were appended despite the failure: %+v", res.Options)
	}
}
