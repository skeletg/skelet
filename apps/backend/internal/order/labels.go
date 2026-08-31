package order

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/ptr"
	shippingpkg "github.com/skeletg/skelet/apps/backend/internal/shipping"
)

// GenerateLabel makes and persists a shipping label for a fulfillment via the order's carrier, stamping the returned tracking number on the
// order when it has none - Errors when the order has no carrier, shipping isn't wired, or the carrier has no Labeler
func (s *Service) GenerateLabel(ctx context.Context, fulfillmentID uuid.UUID) (ShippingLabel, error) {
	if s.shipping == nil {
		return ShippingLabel{}, apperr.New("LABEL_UNAVAILABLE", apperr.ClassUnavailable, "shipping is not configured")
	}
	f, err := s.repo.GetFulfillment(ctx, fulfillmentID)
	if err != nil {
		return ShippingLabel{}, fmt.Errorf("fulfillment not found")
	}
	o, err := s.repo.GetOrderByID(ctx, f.OrderID)
	if err != nil {
		return ShippingLabel{}, err
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
		Items:             rateItemsFromLines(o.Lines),
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
