package sdk

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Reading a shopper's cart, for a plugin that turns one into something else - a quote request, an
// approval, a requisition, a saved list
//
// READ ONLY, and deliberately so. Writing a cart goes through the operations core exposes on the
// graph, which own the pricing, promotion and stock invariants a direct write would step past
type CartLine struct {
	SKU      string
	Name     string
	Quantity float64
	// The line's resolved unit price at read time, which for a company member already reflects any
	// contract rate a price resolver supplied
	UnitPriceCents int64
	// Custom-option picks flattened to "Title: value; Title: value" by core, so a plugin needs no
	// part of the option model to carry personalization through into whatever it creates
	OptionsSummary string
}

// No currency here: a cart's currency belongs to its channel, not to the cart, and duplicating it
// would give a plugin a second copy to disagree with
type Cart struct {
	ID         uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	Lines      []CartLine
}

// The cart is on another store than the request - the same refusal core's own cart resolvers make,
// so a cart id guessed from another channel reads as missing rather than as someone else's basket
var ErrCartNotOnStore = errors.New("sdk: cart is not on this store")

// Returned when the host has not wired cart reads, which is a boot-order bug rather than a runtime state
var ErrCartsUnavailable = errors.New("sdk: carts unavailable")

// The cart, scoped to the request's channel and caller by the host before it is handed over
//
// Scope is enforced host-side, not here, so a plugin cannot forget it. What a plugin still owns is
// WHO may act on the cart it got back - being able to read a cart is not permission to quote it for
// a different customer
func ReadCart(ctx context.Context, cartID uuid.UUID) (Cart, error) {
	h := hostNow()
	if h == nil {
		return Cart{}, ErrCartsUnavailable
	}
	return h.ReadCart(ctx, cartID)
}
