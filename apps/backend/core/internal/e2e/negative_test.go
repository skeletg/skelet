package e2e

import (
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// The gates that must fail closed - each rejection must also leave no residue: no order row
// hinted at here, and above all no stock held for a sale that never happened

func TestPlacementRejectsDisabledPaymentMethod(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	if _, err := h.D.Payment.SetEnabled(h.Ctx, f.Channel.ID, "cod", false); err != nil {
		t.Fatalf("disable cod: %v", err)
	}
	cartID := h.NewGuestCart(t, f, 1)

	if _, err := h.TryPlace(t, f.Channel, cartID); err == nil {
		t.Fatal("placement with a disabled payment method succeeded")
	}
	if got := h.ReservedFor(t, f); got != 0 {
		t.Errorf("reserved = %v after a rejected placement, want 0", got)
	}
}

// A strict channel refuses the address the validator flags, and a lenient one lets it through
func TestPlacementEnforcesStrictAddressValidation(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	badZip := func(in *order.PlaceOrderInput) {
		a := *in.ShippingAddress
		a.PostalCode = "ABCDE"
		in.ShippingAddress = &a
	}

	if _, err := h.TryPlace(t, f.Channel, h.NewGuestCart(t, f, 1), badZip); err != nil {
		t.Fatalf("lenient channel refused a flagged address: %v", err)
	}

	h.setChannelConfig(t, f, "sales/checkout/address_validation_strict", "true")
	reserved := h.ReservedFor(t, f)
	_, err := h.TryPlace(t, f.Channel, h.NewGuestCart(t, f, 1), badZip)
	if code := apperr.CodeOf(err); code != "ADDRESS_INVALID" {
		t.Fatalf("strict placement error = %v (code %q), want ADDRESS_INVALID", err, code)
	}
	if !strings.Contains(err.Error(), "postal code") {
		t.Errorf("error %q should carry the validator's message", err)
	}
	if got := h.ReservedFor(t, f); got != reserved {
		t.Errorf("reserved = %v after a rejected placement, want %v", got, reserved)
	}
	if _, err := h.TryPlace(t, f.Channel, h.NewGuestCart(t, f, 1)); err != nil {
		t.Errorf("strict channel refused a valid address: %v", err)
	}
	h.Settle(t)
}

func TestCartRejectsUnpublishedSKU(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	hidden := h.AddProduct(t, f, 500, false, 10)

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("create guest cart: %v", err)
	}
	_, err = h.D.Cart.AddToCart(h.Ctx, c.ID, nil, hidden.SKU, 1)
	if err == nil {
		t.Fatal("adding an unpublished sku succeeded")
	}
	if !strings.Contains(err.Error(), "not published") {
		t.Errorf("rejection = %v, want the not-published message", err)
	}
}

func TestPlacementRejectsInsufficientStock(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 2, 1000)

	// The add-time stock guard fails open by design, so the cart may accept the quantity - the
	// reservation inside the placement transaction is the gate that must hold
	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("create guest cart: %v", err)
	}
	if _, err := h.D.Cart.AddToCart(h.Ctx, c.ID, nil, f.Product.SKU, 3); err != nil {
		t.Logf("rejected at add-to-cart: %v", err) // also a valid outcome, and placement is moot
		return
	}
	if _, err := h.TryPlace(t, f.Channel, c.ID); err == nil {
		t.Fatal("placed 3 units against a stock of 2")
	}
	if got := h.ReservedFor(t, f); got != 0 {
		t.Errorf("reserved = %v after a rejected placement, want 0 - the failed tx must hold nothing", got)
	}
}

func TestPlacementRejectsCrossChannelCart(t *testing.T) {
	h := newHarness(t)
	a := h.NewStore(t, 10, 1000)
	b := h.NewStore(t, 10, 1000)
	cartOnA := h.NewGuestCart(t, a, 1)

	_, err := h.TryPlace(t, b.Channel, cartOnA)
	if err == nil {
		t.Fatal("placed a channel-A cart under channel B")
	}
	if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "CART_WRONG_STORE" {
		t.Errorf("rejection = %v, want code CART_WRONG_STORE", err)
	}
}

// Two carts race for the last unit - the reservation system's reason to exist. Exactly one
// placement may win, and the loser must leave nothing behind
func TestLastUnitRace(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 1, 1000)
	carts := []struct {
		id  uuid.UUID
		o   order.Order
		err error
	}{
		{id: h.NewGuestCart(t, f, 1)},
		{id: h.NewGuestCart(t, f, 1)},
	}

	var wg sync.WaitGroup
	for i := range carts {
		wg.Go(func() {
			carts[i].o, carts[i].err = h.TryPlace(t, f.Channel, carts[i].id)
		})
	}
	wg.Wait()

	wins := 0
	for _, c := range carts {
		if c.err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d placements won the last unit, want exactly 1 (errs: %v, %v)", wins, carts[0].err, carts[1].err)
	}
	if got := h.ReservedFor(t, f); got != 1 {
		t.Errorf("reserved = %v after the race, want exactly 1", got)
	}
	h.Settle(t)
}
