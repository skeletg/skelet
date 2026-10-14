package front

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	frontmodel "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
)

// An offer's terms for the shopper - nil on a shopper cart
func toCartOfferModel(c cart.Cart) *frontmodel.CartOffer {
	if !c.IsOffer() {
		return nil
	}
	locks := make([]model.CartOfferLock, len(c.Offer.Locks))
	for i, l := range c.Offer.Locks {
		locks[i] = model.CartOfferLock(strings.ToUpper(l))
	}
	return &frontmodel.CartOffer{
		ExpiresAt:      c.Offer.ExpiresAt,
		Message:        c.Offer.Message,
		State:          offerStateModel(c.OfferState(time.Now())),
		Locks:          locks,
		IsGuest:        c.CustomerID == nil,
		PaymentMethods: append([]string{}, c.Offer.PaymentMethods...),
	}
}

func offerStateModel(s cart.OfferState) model.CartOfferState {
	return model.CartOfferState(strings.ToUpper(string(s)))
}

// The console fills in an offer's checkout through these - the storefront's own checkout view, run on the offer's store, so the admin's delivery and payment are exactly what the recipient's checkout accepts

// OfferNeedsShipping says whether the cart has anything to deliver - shipping and delivery locks mean nothing without it
func (r *Resolver) OfferNeedsShipping(ctx context.Context, c cart.Cart) bool {
	skus := make([]string, len(c.Lines))
	for i, l := range c.Lines {
		skus[i] = l.ProductSku
	}
	var profiles map[string]delivery.LineProfile
	if r.DeliverySvc != nil {
		profiles, _ = r.DeliverySvc.LineProfiles(ctx, skus)
	}
	plan := delivery.PlanGroups(cart.DeliveryLines(c.Lines), cart.DeliveryProfiles(c.Lines, profiles), delivery.OfferedKinds(r.offeredOptions(ctx, c.ChannelID)), false)
	return plan.Need != delivery.NeedNone
}

// OfferDeliveryGroups quotes the offer's groups at its checkout as it stands
func (r *Resolver) OfferDeliveryGroups(ctx context.Context, ch *channel.Channel, c cart.Cart) []cart.OfferDeliveryGroup {
	v := r.checkoutView(channel.WithChannel(ctx, ch), c)
	out := make([]cart.OfferDeliveryGroup, 0, len(v.groups))
	for _, g := range v.groups {
		og := cart.OfferDeliveryGroup{Key: g.Key}
		for _, l := range g.Lines {
			og.LineNames = append(og.LineNames, l.Name)
		}
		for _, choice := range g.choices {
			og.Choices = append(og.Choices, cart.OfferDeliveryChoice{
				OptionID:    choice.option.ID,
				OptionName:  choice.option.Name,
				Kind:        choice.option.Kind,
				CarrierSlug: choice.carrierSlug,
				MethodCode:  choice.methodCode,
				MethodName:  choice.methodName,
				PriceCents:  choice.priceCents,
				NeedsPoint:  choice.kind.Needs(delivery.BlockPoint),
			})
		}
		out = append(out, og)
	}
	return out
}

// OfferDeliveryPoints lists the points one of a group's options delivers to, near the offer's address
func (r *Resolver) OfferDeliveryPoints(ctx context.Context, ch *channel.Channel, c cart.Cart, groupKey string, optionID uuid.UUID, search string) ([]delivery.Point, error) {
	g := r.checkoutView(channel.WithChannel(ctx, ch), c).group(groupKey)
	if g == nil {
		return nil, errDeliveryGroupUnknown
	}
	for _, choice := range g.choices {
		if choice.option.ID != optionID {
			continue
		}
		if !choice.kind.Needs(delivery.BlockPoint) || r.DeliverySvc == nil {
			return nil, errDeliveryNoPoint
		}
		return r.DeliverySvc.SearchPoints(ctx, choice.option, delivery.PointQuery{Near: deliveryNear(c.Checkout), Search: search, Items: deliveryItems(g.Lines)})
	}
	return nil, errDeliveryChoice
}

// ApplyOfferDeliveries replaces the offer's deliveries with the admin's - each must be one of its group's choices at the checkout c now holds, and a point one its option delivers to
func (r *Resolver) ApplyOfferDeliveries(ctx context.Context, ch *channel.Channel, c *cart.Cart, in []cart.OfferDeliveryInput) error {
	ctx = channel.WithChannel(ctx, ch)
	v := r.checkoutView(ctx, *c)
	c.Checkout.Deliveries = nil
	for _, sel := range in {
		g := v.group(sel.GroupKey)
		if g == nil {
			return errDeliveryGroupUnknown
		}
		var picked *deliveryChoice
		for i := range g.choices {
			choice := &g.choices[i]
			if choice.option.ID == sel.OptionID && choice.carrierSlug == strings.TrimSpace(sel.CarrierSlug) && choice.methodCode == strings.TrimSpace(sel.MethodCode) {
				picked = choice
				break
			}
		}
		if picked == nil {
			return errDeliveryChoice
		}
		d := cart.DeliverySelection{OptionID: picked.option.ID, CarrierSlug: picked.carrierSlug, MethodCode: picked.methodCode}
		if pid := strings.TrimSpace(sel.PointID); pid != "" {
			if !picked.kind.Needs(delivery.BlockPoint) || r.DeliverySvc == nil {
				return errDeliveryNoPoint
			}
			p, err := r.DeliverySvc.GetPoint(ctx, picked.option, pid)
			if err != nil {
				return err
			}
			d.Point = &p
		}
		c.Checkout.SetDelivery(g.Key, d)
	}
	return nil
}

// OfferPaymentOffered says whether the store takes this payment method for the offer's destination
func (r *Resolver) OfferPaymentOffered(ctx context.Context, c cart.Cart, slug string) bool {
	return r.paymentMethodOffered(ctx, c.ChannelID, slug, paymentCountry(c.Checkout))
}
