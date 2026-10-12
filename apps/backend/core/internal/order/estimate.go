package order

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

type TaxEstimateInput struct {
	Store         *channel.Channel
	CustomerID    *uuid.UUID
	Cart          cart.Cart
	DiscountCents int64
	ShippingCents int64
	CountryCode   string
	RegionCode    string
	PostalCode    string
}

// Same remote-then-core path as placement, so the estimate is what gets charged
func (s *Service) EstimateTax(ctx context.Context, in TaxEstimateInput) (int64, error) {
	var c customer.Customer
	if in.CustomerID != nil && s.customerSvc != nil {
		loaded, err := s.customerSvc.GetByID(ctx, *in.CustomerID)
		if err != nil {
			return 0, fmt.Errorf("load customer: %w", err)
		}
		c = loaded
	}
	var subtotal int64
	for _, l := range in.Cart.Lines {
		subtotal += money.LineTotal(l.UnitPrice, l.Quantity)
	}
	addr := OrderAddress{CountryCode: in.CountryCode, PostalCode: in.PostalCode}
	if in.RegionCode != "" {
		region := in.RegionCode
		addr.State = &region
	}
	cc := &checkoutContext{customer: c, shipping: addr, store: in.Store, cart: in.Cart, subtotal: subtotal}
	discount := max(in.DiscountCents, 0)

	if s.remoteTax != nil {
		if cents, ok, err := s.remoteTax.Tax(ctx, remoteTaxInputFromCheckout(cc, discount, in.ShippingCents)); err == nil && ok {
			return cents, nil
		}
	}
	if s.tax == nil {
		return 0, nil
	}
	return s.computeLineTax(ctx, cc, discount, in.ShippingCents)
}
