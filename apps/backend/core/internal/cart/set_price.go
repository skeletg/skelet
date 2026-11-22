package cart

import (
	"context"
	"errors"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

var (
	ErrLinePriceReasonRequired = errors.New("say why the price is being set")
	ErrLinePriceTooLow         = errors.New("that is below the lowest price that can be set on this line")
	ErrLinePriceQuantity       = errors.New("this price is for fewer of the item")
)

func init() {
	apperr.RegisterSentinel(ErrLinePriceReasonRequired, "CART_LINE_PRICE_REASON_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrLinePriceTooLow, "CART_LINE_PRICE_TOO_LOW", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrLinePriceQuantity, "CART_LINE_PRICE_QUANTITY", apperr.ClassValidation)
}

// PriceQuantityError refuses a shopper more of a line than its set price is for, and says how many that is
type PriceQuantityError struct{ Max float64 }

// Read by the shopper as it stands - the storefronts show a cart refusal's own words
func (e *PriceQuantityError) Error() string {
	return fmt.Sprintf("the price on this line is for up to %v of the item", e.Max)
}
func (e *PriceQuantityError) Unwrap() error { return ErrLinePriceQuantity }
func (e *PriceQuantityError) ErrorExtensions() map[string]any {
	return map[string]any{"maxQuantity": e.Max}
}

// PriceFloorError refuses a price below what the store lets an admin set, and says what the lowest is
type PriceFloorError struct{ FloorCents int64 }

func (e *PriceFloorError) Error() string { return ErrLinePriceTooLow.Error() }
func (e *PriceFloorError) Unwrap() error { return ErrLinePriceTooLow }
func (e *PriceFloorError) ErrorExtensions() map[string]any {
	return map[string]any{"floorCents": e.FloorCents}
}

// shopperPrice holds a price set on a shopper's own cart to its rules. The shopper can change that cart, so the price
// is for no more of the item than the line holds as it is set, and lapses when the store says. It also needs a
// reason, and may take no more off the catalog price than the store allows the admin
func shopperPrice(p LinePrice, line CartLine, in SetLinePriceInput) (LinePrice, error) {
	if p.Reason == "" {
		return LinePrice{}, ErrLinePriceReasonRequired
	}
	if in.MaxCutPercent != nil && p.ListUnitPrice != nil {
		cut := int64(min(max(*in.MaxCutPercent, 0), 100))
		if floor := (*p.ListUnitPrice*(100-cut) + 99) / 100; p.UnitPrice < floor { // rounded up: a cent under is over the cut
			return LinePrice{}, &PriceFloorError{FloorCents: floor}
		}
	}
	quantity := line.Quantity
	p.MaxQuantity, p.ExpiresAt = &quantity, in.HoldsUntil
	return p, nil
}

// setPriceAllows checks a shopper's replacing add against the line it will replace: a set price there is for so many
func (s *Service) setPriceAllows(ctx context.Context, c Cart, sku, configKey string, quantity float64) error {
	lines, err := s.repo.ListCartLinesByCart(ctx, c.ID)
	if err != nil {
		return fmt.Errorf("load cart lines: %w", err)
	}
	for _, l := range lines {
		if l.ProductSku == sku && l.ConfigKey == configKey {
			return l.PriceQuantityErr(quantity)
		}
	}
	return nil
}
