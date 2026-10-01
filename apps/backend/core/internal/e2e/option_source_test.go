package e2e

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

func (h *harness) addLinkedValue(t *testing.T, productID, sourceID uuid.UUID) (catalog.ProductOption, catalog.ProductOptionValue) {
	t.Helper()
	opt, err := h.D.Catalog.AddProductOption(h.Ctx, catalog.AddProductOptionInput{
		ProductID: productID, InputType: catalog.OptionInputDropdown, Locale: "en", Title: "Add-on",
	})
	if err != nil {
		t.Fatalf("add option: %v", err)
	}
	v, err := h.D.Catalog.AddProductOptionValue(h.Ctx, catalog.AddProductOptionValueInput{
		OptionID: opt.ID, Locale: "en", Title: "Linked add-on", PriceValue: 50,
		PriceType: catalog.OptionPriceFixed, SourceProductID: &sourceID, Quantity: 1,
	})
	if err != nil {
		t.Fatalf("add linked value: %v", err)
	}
	return opt, v
}

func (h *harness) addConfigured(t *testing.T, f *store, sku string, optID, valueID uuid.UUID) (cart.Cart, error) {
	t.Helper()
	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	return h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: sku, Quantity: 1,
		Options: []cart.CartLineOptionInput{{OptionID: optID, ValueIDs: []uuid.UUID{valueID}}},
	})
}

// Scenario: a linked value whose source this channel doesn't sell is neither shown nor sellable
func TestLinkedOptionSourceFollowsChannelVisibility(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	src := h.AddProduct(t, f, 700, false, 10)
	opt, val := h.addLinkedValue(t, f.Product.ID, src.ID)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()

	query := fmt.Sprintf(`{ product(id: %q) { options { values { id sourceProduct { id } } } } }`, f.Product.ID)
	source := func() any {
		data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), query)
		if len(errs) > 0 {
			t.Fatalf("product query: %v", errs)
		}
		opts := data["product"].(map[string]any)["options"].([]any)
		return opts[0].(map[string]any)["values"].([]any)[0].(map[string]any)["sourceProduct"]
	}

	if got := source(); got != nil {
		t.Errorf("sourceProduct = %v for a source this channel hides, want null", got)
	}
	if _, err := h.addConfigured(t, f, f.Product.SKU, opt.ID, val.ID); !errors.Is(err, cart.ErrOptionValueUnavailable) {
		t.Errorf("picking an unsold linked value = %v, want ErrOptionValueUnavailable", err)
	}

	h.ListOn(t, f.Channel, src.ID, 900)
	if got := source(); got == nil {
		t.Error("sourceProduct stayed null once the channel sells the source")
	}
	got, err := h.addConfigured(t, f, f.Product.SKU, opt.ID, val.ID)
	if err != nil {
		t.Fatalf("pick a sold linked value: %v", err)
	}
	if got.Lines[0].UnitPrice != 2000+900+50 {
		t.Errorf("unit price = %d, want base 2000 + channel source 900 + markup 50", got.Lines[0].UnitPrice)
	}
}

// Scenario: a percent surcharge is taken of what the channel charges, not the catalog default
func TestPercentSurchargeUsesTheChannelPrice(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.ListOn(t, f.Channel, f.Product.ID, 5000)
	opt, err := h.D.Catalog.AddProductOption(h.Ctx, catalog.AddProductOptionInput{
		ProductID: f.Product.ID, InputType: catalog.OptionInputDropdown, Locale: "en", Title: "Care plan",
	})
	if err != nil {
		t.Fatalf("add option: %v", err)
	}
	v, err := h.D.Catalog.AddProductOptionValue(h.Ctx, catalog.AddProductOptionValueInput{
		OptionID: opt.ID, Locale: "en", Title: "Ten percent", PriceValue: 10, PriceType: catalog.OptionPricePercent, Quantity: 1,
	})
	if err != nil {
		t.Fatalf("add value: %v", err)
	}
	got, err := h.addConfigured(t, f, f.Product.SKU, opt.ID, v.ID)
	if err != nil {
		t.Fatalf("add configured: %v", err)
	}
	if got.Lines[0].UnitPrice != 5500 {
		t.Errorf("unit price = %d, want the 5000 channel price plus 10%% of it", got.Lines[0].UnitPrice)
	}
}
