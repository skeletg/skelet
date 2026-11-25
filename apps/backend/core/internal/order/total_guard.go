package order

import (
	"errors"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// ErrTotalChanged refuses a placement that would charge something other than the total the shopper was shown
var ErrTotalChanged = errors.New("the order total changed since it was shown - check it and place the order again")

func init() {
	apperr.RegisterSentinel(ErrTotalChanged, "CHECKOUT_TOTAL_CHANGED", apperr.ClassConflict)
}

// TotalChangedError is ErrTotalChanged with what the order comes to now. A price, a promotion or a rate can move
// between the page being drawn and the button being pressed, and a plugin can change a figure only at placement, where
// no quote shows it - either way the storefront shows these figures and asks again with the new total
type TotalChangedError struct {
	ExpectedCents int64
	Now           CheckoutQuote
}

func (e *TotalChangedError) Error() string { return ErrTotalChanged.Error() }
func (e *TotalChangedError) Unwrap() error { return ErrTotalChanged }

// ErrorExtensions rides the GraphQL error beside its code, so a storefront reads the figures instead of the message
func (e *TotalChangedError) ErrorExtensions() map[string]any {
	return map[string]any{
		"expectedTotalCents": e.ExpectedCents,
		"totalCents":         e.Now.TotalCents,
		"subtotalCents":      e.Now.SubtotalCents,
		"discountCents":      e.Now.DiscountCents,
		"shippingCents":      e.Now.ShippingCents,
		"feesCents":          feeTotal(e.Now.Fees),
		"taxCents":           e.Now.TaxCents,
	}
}
