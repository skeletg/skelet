package sdk

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// The event type carrying an order's unpaid balance, published inside the placement transaction -
// subscribe with SubscribeSync and type-assert the event to Tendering
const EventOrderTendering = "order.tendering"

// The pre-commit view of what an order still owes, for a plugin implementing a tender core doesn't
// ship - a credit line, a pay-later arrangement, a purchase order against agreed terms
//
// It fires for every placement with a balance, whatever the method, so the FIRST thing a handler does
// is check TenderProvider() against the provider slug it registered and return nil when it doesn't
// match. Core knows no method names beyond its own
//
// Returning an error aborts the placement and rolls the transaction back. Return a UserError so the
// shopper reads why ("your company credit line is short by ..."), not "internal error"
//
// Write through PlacementTx() and nothing else - a write on the pool handle from sdk.Database survives
// a placement that later fails, which is how a reservation outlives the order that justified it
//
//	p.SubscribeSync(sdk.EventOrderTendering, 100, func(ctx context.Context, e any) error {
//	    t, ok := e.(sdk.Tendering)
//	    if !ok || t.TenderProvider() != ProviderSlug {
//	        return nil
//	    }
//	    return reserve(ctx, t.PlacementTx(), t)
//	})
type Tendering interface {
	// The placement transaction - join it, so a reservation commits or rolls back with the order
	PlacementTx() *sql.Tx

	TenderOrderID() uuid.UUID
	// The human-facing number on the order, for a receivable that has to be recognisable to a buyer
	TenderOrderNumber() string
	TenderChannelID() uuid.UUID
	// nil for a guest checkout - a tender keyed on the buyer's identity must REFUSE here rather than
	// fall through, or it becomes an unauthenticated way to spend someone's credit line
	TenderCustomerID() *uuid.UUID
	TenderEmail() string
	// The payment method slug chosen at checkout - the handler's own, or none of its business
	TenderProvider() string
	TenderCurrency() string
	TenderTotalCents() int64
	// What is owed after core's gift-card and store-credit tenders, always > 0
	TenderRemainingCents() int64
}

// The assertion a handler makes, as a function - core pins its own event to Tendering at compile time,
// so a renamed accessor fails core's build rather than silently failing this assertion at checkout
func AssertTendering(_ context.Context, e any) (Tendering, bool) {
	t, ok := e.(Tendering)
	return t, ok
}
