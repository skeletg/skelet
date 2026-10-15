package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
)

// flakyCarrier quotes one method until a test takes it down - registered once for the package and enabled only on the
// stores that ask for it, since new carriers start disabled everywhere
type flakyCarrier struct{ down atomic.Bool }

func (c *flakyCarrier) Quote(context.Context, shipping.RateRequest) ([]shipping.RatedMethod, error) {
	if c.down.Load() {
		return nil, errors.New("carrier API timed out")
	}
	return []shipping.RatedMethod{{Code: "express", Name: "Express", PriceCents: 900}}, nil
}

const flakySlug = "e2e-flaky"

var (
	flaky         = &flakyCarrier{}
	registerFlaky sync.Once
)

// Scenario: a carrier that can't be asked isn't a carrier that stopped offering the method - re-saving a step keeps the
// shopper's choice, and placement refuses as unavailable rather than calling it not offered or charging another method
func TestCheckoutKeepsTheChoiceWhenItsCarrierCantBeAsked(t *testing.T) {
	registerFlaky.Do(func() {
		shipping.RegisterCarrier(shipping.CarrierRegistration{Slug: flakySlug, Carrier: flaky, DisplayName: "Flaky"})
	})
	flaky.down.Store(false)
	t.Cleanup(func() { flaky.down.Store(false) })

	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	if err := h.D.Shipping.SyncCarriers(h.Ctx, []uuid.UUID{f.Channel.ID}); err != nil {
		t.Fatalf("sync carriers: %v", err)
	}
	if _, err := h.D.Shipping.SetCarrierEnabled(h.Ctx, f.Channel.ID, flakySlug, true); err != nil {
		t.Fatalf("enable the flaky carrier: %v", err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	email := "flaky-" + uuid.NewString()[:8] + "@e2e.test"

	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: %q, code: "express")`, cartID, flakySlug))
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))

	flaky.down.Store(true)
	// Re-saving the address re-checks the method against the carriers - an outage mustn't clear it
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)))
	// ...nor may picking it again be answered "not offered"
	msg := h.gqlCheckoutRefused(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: %q, code: "express")`, cartID, flakySlug))
	if strings.Contains(msg, "isn't available for this address") {
		t.Errorf("choosing a method whose carrier is down = %q, want the quote failure, not SHIPPING_METHOD_UNAVAILABLE", msg)
	}
	_, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if len(errs) == 0 {
		t.Fatal("placeOrder with the chosen carrier down was accepted - it would have charged a different method")
	}
	if strings.Contains(errs[0], "isn't available") || strings.Contains(errs[0], "choose a delivery") {
		t.Errorf("placeOrder with the carrier down = %q, want the quote failure", errs[0])
	}
	t.Logf("refusal while the carrier is down: %q / %q", msg, errs[0])

	flaky.down.Store(false)
	co := h.gqlCartCheckout(t, ts, f, cartID)
	if m, _ := co["shippingMethod"].(map[string]any); m == nil || m["carrierSlug"] != flakySlug || m["code"] != "express" {
		t.Fatalf("shipping method once the carrier is back = %v, want the shopper's choice kept through the outage", co["shippingMethod"])
	}
	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder once the carrier is back: %v", errs)
	}
	row, err := h.D.Queries.GetOrderByID(h.Ctx, uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string)))
	if err != nil {
		t.Fatalf("order row: %v", err)
	}
	if row.ShippingCarrierSlug.String != flakySlug || row.ShippingAmount != 900 {
		t.Errorf("order carrier=%q shipping=%d, want %s at 900", row.ShippingCarrierSlug.String, row.ShippingAmount, flakySlug)
	}
	h.Settle(t)
}
