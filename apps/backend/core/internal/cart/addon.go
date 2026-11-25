package cart

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

var (
	ErrLineFollowsParent = errors.New("this item goes with another in the cart - change that one instead")
	ErrAddOnRequired     = errors.New("this item is part of another in the cart and can't be removed on its own - remove that one instead")
)

func init() {
	apperr.RegisterSentinel(ErrLineFollowsParent, "CART_LINE_FOLLOWS_PARENT", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrAddOnRequired, "CART_ADD_ON_REQUIRED", apperr.ClassValidation)
}

// IsAddOn is whether the line belongs to another - picked as one of its options and sold as a line of its own
func (l CartLine) IsAddOn() bool { return l.ParentLineID != nil }

// removeAddOn takes an add-on line out of the cart together with the pick it stood for on its parent. An option
// the product needs a choice for can't lose its only one this way - the shopper removes the item itself, or picks again
func (s *Service) removeAddOn(ctx context.Context, c Cart, line CartLine) error {
	if line.ParentLineID == nil || line.ParentValueID == nil {
		return s.repo.RemoveCartLine(ctx, line.ID)
	}
	parent, err := s.findHydratedLine(ctx, c.ID, *line.ParentLineID)
	if err != nil {
		return err
	}
	if s.products != nil {
		res, rerr := s.products.ResolveProductOptions(ctx, c.ChannelID, parent.ProductSku, c.CustomerID)
		if rerr != nil {
			return fmt.Errorf("resolve product options for %q: %w", parent.ProductSku, rerr)
		}
		if addOnRequired(res, parent, *line.ParentValueID) {
			return ErrAddOnRequired
		}
	}
	return s.inTx(ctx, func(r Repository) error { return r.RemoveAddOnLine(ctx, line) })
}

// Whether the pick is the only one left on an option the product requires a choice for
func addOnRequired(res ProductOptionsResolution, parent CartLine, valueID uuid.UUID) bool {
	for _, opt := range res.Options {
		owns := false
		for _, v := range opt.Values {
			if v.ID == valueID {
				owns = true
			}
		}
		if !owns {
			continue
		}
		if !opt.IsRequired {
			return false
		}
		picks := 0
		for _, o := range parent.Options {
			if o.ProductOptionID == opt.ID {
				picks++
			}
		}
		return picks <= 1
	}
	return false
}

// What the add-ons take per unit of the line they go with, for a stock guard weighing the add - the lines they become
// reserve it themselves once they exist
func addOnStockParts(addOns []LineAddOn) []CartLineStockPart {
	out := make([]CartLineStockPart, 0, len(addOns))
	for _, a := range addOns {
		out = append(out, CartLineStockPart{ProductID: a.ProductID, Quantity: a.PerParent, Source: StockPartOption})
	}
	return out
}

// The add-ons a line carries, read back from the lines under it - for copying the pair to another cart
func addOnsOf(parent CartLine, lines []CartLine) []LineAddOn {
	var out []LineAddOn
	for _, l := range lines {
		if l.ParentLineID == nil || *l.ParentLineID != parent.ID || l.ParentValueID == nil || l.PerParent == nil {
			continue
		}
		out = append(out, LineAddOn{ValueID: *l.ParentValueID, ProductSku: l.ProductSku, ProductName: l.ProductName, UnitPrice: l.UnitPrice, PerParent: *l.PerParent, NoPromotions: l.NoPromotions})
	}
	return out
}
