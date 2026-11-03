package order

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	shippingpkg "github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// GenerateLabel makes and persists a shipping label for a fulfillment via the order's carrier, stamping the returned tracking number on the
// order when it has none - Errors when the order has no carrier, shipping isn't wired, or the carrier has no Labeler
// Every call buys a label from the carrier, so a second one for the same shipment needs force
func (s *Service) GenerateLabel(ctx context.Context, fulfillmentID uuid.UUID, force bool) (ShippingLabel, error) {
	shell, err := s.repo.GetFulfillment(ctx, fulfillmentID)
	if err != nil {
		return ShippingLabel{}, err
	}
	o, err := s.repo.GetOrderByID(ctx, shell.OrderID)
	if err != nil {
		return ShippingLabel{}, err
	}
	f, err := s.fulfillmentWithLines(ctx, o.ID, fulfillmentID)
	if err != nil {
		return ShippingLabel{}, err
	}
	if f.Status == FulfillmentCancelled {
		return ShippingLabel{}, apperr.New("LABEL_FULFILLMENT_CANCELLED", apperr.ClassConflict, "a cancelled shipment can't get a label")
	}
	if len(f.Labels) > 0 && !force {
		return ShippingLabel{}, apperr.New("LABEL_EXISTS", apperr.ClassConflict,
			"this shipment already has a label - buying another one charges the carrier again")
	}
	if s.shipping == nil {
		return ShippingLabel{}, apperr.New("LABEL_UNAVAILABLE", apperr.ClassUnavailable, "shipping is not configured")
	}
	slug := ""
	if o.ShippingCarrierSlug != nil {
		slug = *o.ShippingCarrierSlug
	}
	if slug == "" {
		return ShippingLabel{}, apperr.New("LABEL_NO_CARRIER", apperr.ClassConflict,
			"order has no shipping carrier to generate a label from")
	}

	res, err := s.shipping.GenerateLabel(ctx, slug, shippingpkg.LabelRequest{
		OrderNumber:       o.Number,
		ChannelID:         o.ChannelID,
		CarrierSlug:       slug,
		MethodCode:        ptr.Deref(o.ShippingMethodCode),
		ShipTo:            orderAddressToShipping(o.ShippingAddress),
		Items:             shipmentItems(o, f),
		InsuredValueCents: f.InsuredValueCents,
	})
	if err != nil {
		return ShippingLabel{}, err
	}
	if res.Format == "" {
		res.Format = "pdf"
	}
	label, err := s.repo.CreateShippingLabel(ctx, ShippingLabel{
		FulfillmentID:  fulfillmentID,
		CarrierSlug:    slug,
		Format:         res.Format,
		TrackingNumber: res.TrackingNumber,
		Doc:            res.Doc,
	})
	if err != nil {
		return ShippingLabel{}, err
	}
	// A label call often books the shipment + assigns tracking - stamp it on the order when it has none (non-fatal)
	if res.TrackingNumber != "" && o.TrackingNumber == "" {
		if _, err := s.repo.UpdateOrderTracking(ctx, o.ID, o.Carrier, res.TrackingNumber); err != nil {
			log.Printf("[order] stamp tracking from label for order %s: %v", o.ID, err)
		}
	}
	return label, nil
}

// orderAddressToShipping maps an order address to the carrier ship-to shape
func orderAddressToShipping(a OrderAddress) shippingpkg.Address {
	region := ""
	if a.State != nil {
		region = *a.State
	}
	return shippingpkg.Address{
		FirstName:   a.FirstName,
		LastName:    a.LastName,
		Company:     ptr.Deref(a.Company),
		Street1:     a.Street1,
		Street2:     ptr.Deref(a.Street2),
		City:        a.City,
		Region:      region,
		PostalCode:  a.PostalCode,
		CountryCode: a.CountryCode,
		Phone:       ptr.Deref(a.Phone),
	}
}

func (s *Service) fulfillmentWithLines(ctx context.Context, orderID, fulfillmentID uuid.UUID) (Fulfillment, error) {
	all, err := s.repo.ListFulfillments(ctx, orderID)
	if err != nil {
		return Fulfillment{}, err
	}
	for _, f := range all {
		if f.ID == fulfillmentID {
			return f, nil
		}
	}
	return Fulfillment{}, ErrFulfillmentNotFound
}

// What is actually in this box - listing every order line declared and weighed goods a partial shipment never carried
func shipmentItems(o Order, f Fulfillment) []shippingpkg.RateItem {
	byID := make(map[uuid.UUID]OrderLine, len(o.Lines))
	for _, l := range o.Lines {
		byID[l.ID] = l
	}
	out := make([]shippingpkg.RateItem, 0, len(f.Lines))
	for _, fl := range f.Lines {
		if l, ok := byID[fl.OrderLineID]; ok {
			out = append(out, shippingpkg.LineItems(shippingpkg.RateItem{SKU: l.ProductSku, Quantity: fl.Quantity, UnitPriceCents: l.UnitPrice}, l.Quantity, orderLineParts(l))...)
		}
	}
	return out
}
