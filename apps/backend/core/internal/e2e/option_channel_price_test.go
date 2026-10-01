package e2e

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// Scenario: a FIXED surcharge charges its per-channel amount, falls back to the base only in the product's currency, and is refused elsewhere - PERCENT and zero are never refused, and the storefront shows exactly what the cart charges
func TestOptionSurchargeFollowsTheChannelCurrency(t *testing.T) {
	h := newHarness(t)
	usd := h.NewStore(t, 10, 2000)
	eur := h.NewStoreWith(t, 0, 1, "EUR")
	h.ListOn(t, eur.Channel, usd.Product.ID, 1800)
	if _, err := h.D.Inventory.SyncProductTracking(h.Ctx, usd.Product.ID, false); err != nil {
		t.Fatalf("untrack: %v", err)
	}
	sku := usd.Product.SKU
	opt, err := h.D.Catalog.AddProductOption(h.Ctx, catalog.AddProductOptionInput{
		ProductID: usd.Product.ID, InputType: catalog.OptionInputDropdown, Locale: "en", Title: "Finish",
	})
	if err != nil {
		t.Fatalf("add option: %v", err)
	}
	value := func(title string, price int32, pt catalog.ProductOptionPriceType) catalog.ProductOptionValue {
		v, err := h.D.Catalog.AddProductOptionValue(h.Ctx, catalog.AddProductOptionValueInput{
			OptionID: opt.ID, Locale: "en", Title: title, PriceValue: price, PriceType: pt, Quantity: 1,
		})
		if err != nil {
			t.Fatalf("add value %s: %v", title, err)
		}
		return v
	}
	fixed := value("Gloss", 500, catalog.OptionPriceFixed)
	pct := value("Matte", 10, catalog.OptionPricePercent)
	free := value("Plain", 0, catalog.OptionPriceFixed)

	unit := func(f *store, v catalog.ProductOptionValue) (int64, error) {
		c, err := h.addConfigured(t, f, sku, opt.ID, v.ID)
		if err != nil {
			return 0, err
		}
		return c.Lines[0].UnitPrice, nil
	}
	want := func(f *store, v catalog.ProductOptionValue, price int64) {
		t.Helper()
		got, err := unit(f, v)
		if err != nil || got != price {
			t.Errorf("%s on %s = (%d, %v), want %d", v.Title, f.Channel.CurrencyCode, got, err, price)
		}
	}

	want(usd, fixed, 2500)
	if _, err := unit(eur, fixed); !errors.Is(err, cart.ErrOptionPriceUnavailable) {
		t.Errorf("USD surcharge on the EUR channel = %v, want ErrOptionPriceUnavailable", err)
	}
	want(eur, pct, 1980)
	want(eur, free, 1800)

	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	shown := func(f *store) map[string]float64 {
		t.Helper()
		data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`{ product(id: %q) { options { values { id priceValue } } } }`, usd.Product.ID))
		if len(errs) > 0 {
			t.Fatalf("product query: %v", errs)
		}
		out := map[string]float64{}
		for _, v := range data["product"].(map[string]any)["options"].([]any)[0].(map[string]any)["values"].([]any) {
			m := v.(map[string]any)
			out[m["id"].(string)] = m["priceValue"].(float64)
		}
		return out
	}
	if got := shown(eur); len(got) != 2 || got[pct.ID.String()] != 10 {
		t.Errorf("EUR storefront values = %v, want the unpriced Gloss left out", got)
	}

	override := int32(450)
	if _, err := h.D.Catalog.SetProductOptionValueChannelPrice(h.Ctx, fixed.ID, eur.Channel.ID, &override); err != nil {
		t.Fatalf("set EUR surcharge: %v", err)
	}
	want(eur, fixed, 2250)
	want(usd, fixed, 2500)
	if got := shown(eur)[fixed.ID.String()]; got != 450 {
		t.Errorf("EUR storefront Gloss = %v, want the 450 override", got)
	}
	if got := shown(usd)[fixed.ID.String()]; got != 500 {
		t.Errorf("USD storefront Gloss = %v, want the 500 base", got)
	}

	if _, err := h.D.Catalog.SetProductOptionValueChannelPrice(h.Ctx, pct.ID, eur.Channel.ID, &override); !errors.Is(err, catalog.ErrOptionChannelPriceNotFixed) {
		t.Errorf("override on a percent value = %v, want ErrOptionChannelPriceNotFixed", err)
	}
	huge := int32(catalog.MaxOptionChannelSurcharge + 1)
	if _, err := h.D.Catalog.SetProductOptionValueChannelPrice(h.Ctx, fixed.ID, eur.Channel.ID, &huge); !errors.Is(err, catalog.ErrOptionChannelPriceRange) {
		t.Errorf("override past the cap = %v, want ErrOptionChannelPriceRange", err)
	}
	unlisted := h.NewStoreWith(t, 0, 1, "GBP")
	if _, err := h.D.Catalog.SetProductOptionValueChannelPrice(h.Ctx, fixed.ID, unlisted.Channel.ID, &override); !errors.Is(err, catalog.ErrOptionChannelNotListed) {
		t.Errorf("override on an unlisted channel = %v, want ErrOptionChannelNotListed", err)
	}
	if _, err := h.D.Catalog.SetProductOptionValueChannelPrice(h.Ctx, fixed.ID, eur.Channel.ID, nil); err != nil {
		t.Fatalf("clear EUR surcharge: %v", err)
	}
	if _, err := unit(eur, fixed); !errors.Is(err, cart.ErrOptionPriceUnavailable) {
		t.Errorf("after clearing the override = %v, want ErrOptionPriceUnavailable again", err)
	}
}
