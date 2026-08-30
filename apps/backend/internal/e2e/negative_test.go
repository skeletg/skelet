package e2e

import (
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/apperr"
	"github.com/skeletg/skelet/internal/cart"
	"github.com/skeletg/skelet/internal/order"
)

// The gates that must fail closed - each rejection must also leave no residue: no order row
// hinted at here, and above all no stock held for a sale that never happened

func TestPlacementRejectsDisabledPaymentMethod(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 1000)
	if _, err := h.d.Payment.SetEnabled(h.ctx, f.Channel.ID, "cod", false); err != nil {
		t.Fatalf("disable cod: %v", err)
	}
	cartID := h.newGuestCart(t, f, 1)

	if _, err := h.tryPlace(t, f.Channel, cartID); err == nil {
		t.Fatal("placement with a disabled payment method succeeded")
	}
	if got := h.reservedFor(t, f); got != 0 {
		t.Errorf("reserved = %v after a rejected placement, want 0", got)
	}
}

func TestCartRejectsUnpublishedSKU(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 1000)
	hidden := h.addProduct(t, f, 500, false, 10)

	c, err := h.d.Cart.CreateGuestCart(h.ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("create guest cart: %v", err)
	}
	_, err = h.d.Cart.AddToCart(h.ctx, c.ID, nil, cart.AddCartLineInput{ProductSku: hidden.SKU, Quantity: 1})
	if err == nil {
		t.Fatal("adding an unpublished sku succeeded")
	}
	if !strings.Contains(err.Error(), "not published") {
		t.Errorf("rejection = %v, want the not-published message", err)
	}
}

func TestPlacementRejectsInsufficientStock(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 2, 1000)

	// The add-time stock guard fails open by design, so the cart may accept the quantity - the
	// reservation inside the placement transaction is the gate that must hold
	c, err := h.d.Cart.CreateGuestCart(h.ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("create guest cart: %v", err)
	}
	if _, err := h.d.Cart.AddToCart(h.ctx, c.ID, nil, cart.AddCartLineInput{ProductSku: f.Product.SKU, Quantity: 3}); err != nil {
		t.Logf("rejected at add-to-cart: %v", err) // also a valid outcome, and placement is moot
		return
	}
	if _, err := h.tryPlace(t, f.Channel, c.ID); err == nil {
		t.Fatal("placed 3 units against a stock of 2")
	}
	if got := h.reservedFor(t, f); got != 0 {
		t.Errorf("reserved = %v after a rejected placement, want 0 - the failed tx must hold nothing", got)
	}
}

func TestPlacementRejectsCrossChannelCart(t *testing.T) {
	h := newHarness(t)
	a := h.newStore(t, 10, 1000)
	b := h.newStore(t, 10, 1000)
	cartOnA := h.newGuestCart(t, a, 1)

	_, err := h.tryPlace(t, b.Channel, cartOnA)
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
	f := h.newStore(t, 1, 1000)
	carts := []struct {
		id  uuid.UUID
		o   order.Order
		err error
	}{
		{id: h.newGuestCart(t, f, 1)},
		{id: h.newGuestCart(t, f, 1)},
	}

	var wg sync.WaitGroup
	for i := range carts {
		wg.Go(func() {
			carts[i].o, carts[i].err = h.tryPlace(t, f.Channel, carts[i].id)
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
	if got := h.reservedFor(t, f); got != 1 {
		t.Errorf("reserved = %v after the race, want exactly 1", got)
	}
	h.settle(t)
}
