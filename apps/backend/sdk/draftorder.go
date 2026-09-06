package sdk

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Turning an agreed set of lines into a draft order, for a plugin that negotiated them somewhere core
// cannot see - an accepted quote, an approved requisition, a re-order built from history
//
// A DRAFT, deliberately: it holds no stock, authorizes no payment and fires no placement events. An
// operator reviews and places it through the normal draft-order console, so a plugin can propose an
// order without being trusted to commit one
//
// Addresses are the customer's saved defaults, resolved by core - a plugin has no business carrying
// address validation, and a customer with no saved address is an error the operator has to fix
type DraftOrderLine struct {
	SKU      string
	Quantity float64
	// nil prices at catalog. Set it for a negotiated rate the catalog does not know about - the whole
	// reason a quote exists
	UnitPriceCents *int64
}

type DraftOrderRequest struct {
	ChannelID uuid.UUID
	// Required. A draft raised by a plugin is always for a known customer, since its addresses and its
	// pricing both key on one
	CustomerID uuid.UUID
	Lines      []DraftOrderLine
	// Lands on the order's customer note, which is where a quote reference or an options summary
	// belongs when the line itself cannot carry it
	Note string
	// Bypasses the required-options gate for lines whose personalization is recorded elsewhere, e.g.
	// summarised into Note when the quote was raised
	SkipRequiredOptionCheck bool
}

// What the draft became - enough to link to it and show it, not the whole order, since a plugin that
// wants the order should read it through the graph like any other client
type DraftOrderResult struct {
	OrderID      uuid.UUID
	Number       string
	Status       string
	TotalCents   int64
	CurrencyCode string
}

// Returned when the host has not wired draft orders, which is a boot-order bug rather than a runtime
// state - resolve it on first use, never in init()
var ErrDraftOrdersUnavailable = errors.New("sdk: draft orders unavailable")

// Stages a draft order and returns what it became
//
//	res, err := sdk.CreateDraftOrder(ctx, sdk.DraftOrderRequest{...})
func CreateDraftOrder(ctx context.Context, req DraftOrderRequest) (DraftOrderResult, error) {
	h := hostNow()
	if h == nil {
		return DraftOrderResult{}, ErrDraftOrdersUnavailable
	}
	return h.CreateDraftOrder(ctx, req)
}
