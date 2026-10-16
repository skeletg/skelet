package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) AdminCreateCart(ctx context.Context, channelID string, customerID *string) (*model.AdminCart, error) {
	chID, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", chID, nil); err != nil {
		return nil, err
	}
	if err := r.requireCustomerOnChannel(ctx, parseOptionalUUID(customerID), chID); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.AdminCreateCart(ctx, chID, parseOptionalUUID(customerID))
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, c), nil
}

func (r *mutationResolver) AdminAddCartLine(ctx context.Context, cartID string, sku string, quantity float64, options []*model.AdminCartLineOptionInput) (*model.AdminCart, error) {
	id, err := r.cartForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	// Picks route through the configured add - same server-side validation, pricing and snapshot contract as the storefront path
	if len(options) > 0 {
		picks := make([]cart.CartLineOptionInput, 0, len(options))
		for _, o := range options {
			if o == nil {
				continue
			}
			oid, perr := parseID(o.OptionID, "option")
			if perr != nil {
				return nil, perr
			}
			in := cart.CartLineOptionInput{OptionID: oid}
			if o.TextValue != nil {
				in.TextValue = *o.TextValue
			}
			for _, vid := range o.ValueIds {
				v, verr := parseID(vid, "option value")
				if verr != nil {
					return nil, verr
				}
				in.ValueIDs = append(in.ValueIDs, v)
			}
			if o.FileAssetID != nil {
				fid, ferr := parseID(*o.FileAssetID, "file asset")
				if ferr != nil {
					return nil, ferr
				}
				in.FileAssetID = &fid
			}
			picks = append(picks, in)
		}
		c, err := r.CartSvc.AdminAddConfiguredToCart(ctx, id, cart.AddConfiguredToCartInput{ProductSku: sku, Quantity: quantity, Options: picks})
		if err != nil {
			return nil, err
		}
		return r.mapCart(ctx, c), nil
	}
	c, err := r.CartSvc.AdminAddToCart(ctx, id, sku, quantity)
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, c), nil
}

func (r *mutationResolver) AdminUpdateCartLine(ctx context.Context, cartID string, lineID string, quantity float64) (*model.AdminCart, error) {
	id, err := r.cartForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	lid, err := parseID(lineID, "cart line")
	if err != nil {
		return nil, err
	}
	c, err := r.CartSvc.AdminUpdateCartLine(ctx, id, lid, quantity)
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, c), nil
}

func (r *mutationResolver) AdminRemoveCartLine(ctx context.Context, cartID string, lineID string) (*model.AdminCart, error) {
	id, err := r.cartForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	lid, err := parseID(lineID, "cart line")
	if err != nil {
		return nil, err
	}
	c, err := r.CartSvc.AdminRemoveFromCart(ctx, id, lid)
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, c), nil
}

func (r *mutationResolver) AdminClearCart(ctx context.Context, cartID string) (*model.AdminCart, error) {
	id, err := r.cartForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	c, err := r.CartSvc.AdminClearCart(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, c), nil
}

func (r *mutationResolver) AdminDuplicateCart(ctx context.Context, cartID string, targetCustomerID *string) (*model.AdminCart, error) {
	id, err := r.cartForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	src, err := r.CartSvc.Repo().GetCartByID(ctx, id)
	if err != nil {
		return nil, errNotFound("cart")
	}
	if err := r.requireCustomerOnChannel(ctx, parseOptionalUUID(targetCustomerID), src.ChannelID); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.AdminDuplicateCart(ctx, id, parseOptionalUUID(targetCustomerID))
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, c), nil
}

func (r *mutationResolver) AdminCreateCartOffer(ctx context.Context, input model.AdminCreateCartOfferInput) (*model.AdminCart, error) {
	chID, err := parseID(input.ChannelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", chID, nil); err != nil {
		return nil, err
	}
	customerID := parseOptionalUUID(input.CustomerID)
	if err := r.requireCustomerOnChannel(ctx, customerID, chID); err != nil {
		return nil, err
	}
	expires := r.defaultOfferExpiry(ctx, chID)
	if input.ExpiresAt != nil {
		expires = *input.ExpiresAt
	}
	in := cart.CreateOfferInput{ChannelID: chID, CustomerID: customerID, ExpiresAt: expires}
	if customerID == nil && input.Email != nil {
		in.Email = *input.Email
	}
	if claims, ok := auth.AdminFromContext(ctx); ok && claims.AdminID != uuid.Nil {
		in.CreatedBy = &claims.AdminID
	}
	c, err := r.CartSvc.AdminCreateOffer(ctx, in)
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, c), nil
}

func (r *mutationResolver) AdminUpdateCartOffer(ctx context.Context, cartID string, input model.AdminUpdateCartOfferInput) (*model.AdminCart, error) {
	c, err := r.offerForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	ships := r.offerNeedsShipping(ctx, c)
	settings := cart.OfferSettings{ExpiresAt: input.ExpiresAt, Message: input.Message, ShowInAccount: input.ShowInAccount, Locks: lockSlugs(input.Locks)}
	if settings.PaymentMethods, err = r.offerPaymentMethods(ctx, c.ChannelID, input.PaymentMethods); err != nil {
		return nil, err
	}
	input.PaymentMethods = settings.PaymentMethods
	ch, err := r.offerChannel(ctx, c.ChannelID)
	if err != nil {
		return nil, err
	}
	if err := r.CartSvc.AdminUpdateOffer(ctx, c.ID, settings, ships, func(next *cart.Cart) error {
		return r.applyOfferCheckout(ctx, ch, next, input, ships)
	}); err != nil {
		return nil, err
	}
	return r.reloadAdminCart(ctx, c.ID)
}

func (r *mutationResolver) AdminSetCartLinePrice(ctx context.Context, cartID string, lineID string, unitPrice *int32) (*model.AdminCart, error) {
	c, err := r.offerForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	lid, err := parseID(lineID, "cart line")
	if err != nil {
		return nil, err
	}
	var price *int64
	if unitPrice != nil {
		p := int64(*unitPrice)
		price = &p
	}
	out, err := r.CartSvc.AdminSetLinePrice(ctx, c.ID, lid, price)
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, out), nil
}

func (r *mutationResolver) AdminAddCustomCartLine(ctx context.Context, cartID string, input model.AdminCustomCartLineInput) (*model.AdminCart, error) {
	c, err := r.offerForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	in := cart.CustomLineInput{Name: input.Name, UnitPrice: int64(input.UnitPrice), Quantity: input.Quantity}
	if input.TaxClass != nil {
		in.TaxClass = *input.TaxClass
	}
	out, err := r.CartSvc.AdminAddCustomLine(ctx, c.ID, in)
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, out), nil
}

func (r *mutationResolver) AdminSendCartOffer(ctx context.Context, cartID string) (*model.AdminCart, error) {
	c, err := r.offerForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	if r.OfferMail == nil {
		return nil, errOfferNoMailer
	}
	c, err = r.CartSvc.AdminOfferForSend(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	m, complete, err := r.offerMail(ctx, c)
	if err != nil {
		return nil, err
	}
	if !complete {
		return nil, errOfferNoLink
	}
	if m.To == "" {
		return nil, errOfferNoAddress
	}
	if err := r.OfferMail.SendCartOffer(ctx, m); err != nil {
		return nil, err
	}
	if err := r.CartSvc.MarkOfferSent(ctx, c.ID, m.To); err != nil {
		return nil, err
	}
	return r.reloadAdminCart(ctx, c.ID)
}

func (r *mutationResolver) AdminRegenerateCartOfferLink(ctx context.Context, cartID string) (*model.AdminCart, error) {
	c, err := r.offerForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	out, err := r.CartSvc.AdminRegenerateOfferLink(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, out), nil
}

func (r *mutationResolver) AdminRevokeCartOffer(ctx context.Context, cartID string) (*model.AdminCart, error) {
	c, err := r.offerForAdmin(ctx, cartID)
	if err != nil {
		return nil, err
	}
	out, err := r.CartSvc.AdminRevokeOffer(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, out), nil
}

func (r *queryResolver) Carts(ctx context.Context, channelID string, customerID *string, status *string, kind *model.CartKind, abandoned *bool, search *string, limit *int32, offset *int32) (*model.CartList, error) {
	chID, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.view", chID, nil); err != nil {
		return nil, err
	}
	f := cart.CartFilter{
		ChannelID:  chID,
		CustomerID: parseOptionalUUID(customerID),
		Status:     status,
		Kind:       cartKindFilter(kind),
		Search:     search,
		Limit:      50,
		Offset:     0,
	}
	if limit != nil && *limit > 0 {
		f.Limit = min(*limit, maxCartListLimit)
	}
	if offset != nil && *offset > 0 {
		f.Offset = *offset
	}
	if abandoned != nil && *abandoned {
		active := "active"
		f.Status = &active
		cutoff := time.Now().Add(-cartAbandonedAfter)
		f.UpdatedBefore = &cutoff
	}
	repo := r.CartSvc.Repo()
	items, err := repo.ListCarts(ctx, f)
	if err != nil {
		return nil, err
	}
	total, err := repo.CountCarts(ctx, f)
	if err != nil {
		return nil, err
	}
	currency := r.channelCurrency(ctx, chID)
	out := make([]*model.CartListItem, len(items))
	for i, it := range items {
		out[i] = toCartListItemModel(it, currency)
	}
	return &model.CartList{Items: out, TotalCount: total}, nil
}

func (r *queryResolver) Cart(ctx context.Context, id string) (*model.AdminCart, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "orders.view"); err != nil {
		return nil, err
	}
	cartID, err := parseID(id, "cart")
	if err != nil {
		return nil, err
	}
	repo := r.CartSvc.Repo()
	c, err := repo.GetCartByID(ctx, cartID)
	if err != nil {
		return nil, errNotFound("cart")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.view", c.ChannelID, nil); err != nil {
		return nil, err
	}
	lines, err := repo.ListCartLinesByCart(ctx, cartID)
	if err != nil {
		return nil, err
	}
	c.Lines = lines
	return r.mapCart(ctx, c), nil
}

func (r *queryResolver) AdminCartDeliveryGroups(ctx context.Context, cartID string) ([]*model.AdminCartDeliveryGroup, error) {
	c, ch, err := r.offerForView(ctx, cartID)
	if err != nil {
		return nil, err
	}
	out := []*model.AdminCartDeliveryGroup{}
	if r.OfferCheckout == nil {
		return out, nil
	}
	for _, g := range r.OfferCheckout.OfferDeliveryGroups(ctx, ch, c) {
		mg := &model.AdminCartDeliveryGroup{Key: g.Key, Lines: g.LineNames, Choices: []*model.AdminCartDeliveryChoice{}}
		if mg.Lines == nil {
			mg.Lines = []string{}
		}
		for _, choice := range g.Choices {
			mg.Choices = append(mg.Choices, &model.AdminCartDeliveryChoice{
				OptionID: choice.OptionID.String(), OptionName: choice.OptionName, Kind: choice.Kind,
				CarrierSlug: choice.CarrierSlug, MethodCode: choice.MethodCode, MethodName: choice.MethodName,
				Price: int32(choice.PriceCents), NeedsPoint: choice.NeedsPoint,
			})
		}
		out = append(out, mg)
	}
	return out, nil
}

func (r *queryResolver) AdminCartOfferPaymentMethods(ctx context.Context, cartID string) ([]*model.AdminCartOfferPaymentMethod, error) {
	c, _, err := r.offerForView(ctx, cartID)
	if err != nil {
		return nil, err
	}
	out := []*model.AdminCartOfferPaymentMethod{}
	if r.PaymentSvc == nil {
		return out, nil
	}
	methods, err := r.PaymentSvc.ListEnabledPaymentMethods(ctx, c.ChannelID)
	if err != nil {
		return nil, err
	}
	for _, m := range methods {
		name := m.DisplayName
		if name == "" {
			name = m.ProviderSlug
		}
		out = append(out, &model.AdminCartOfferPaymentMethod{Slug: m.ProviderSlug, DisplayName: name})
	}
	return out, nil
}

func (r *queryResolver) AdminCartOfferEmailPreview(ctx context.Context, cartID string) (*model.AdminCartOfferEmailPreview, error) {
	c, _, err := r.offerForView(ctx, cartID)
	if err != nil {
		return nil, err
	}
	if r.OfferMail == nil {
		return nil, errOfferNoMailer
	}
	m, complete, err := r.offerMail(ctx, c)
	if err != nil {
		return nil, err
	}
	p, err := r.OfferMail.PreviewCartOffer(ctx, m)
	if err != nil {
		return nil, err
	}
	return &model.AdminCartOfferEmailPreview{To: m.To, Subject: p.Subject, HTML: p.HTML, Text: p.Text, Link: m.Link, LinkComplete: complete}, nil
}

func (r *queryResolver) AdminCartDeliveryPoints(ctx context.Context, cartID string, groupKey string, optionID string, search *string) ([]*model.AdminCartDeliveryPoint, error) {
	c, ch, err := r.offerForView(ctx, cartID)
	if err != nil {
		return nil, err
	}
	oid, err := parseID(optionID, "delivery option")
	if err != nil {
		return nil, err
	}
	out := []*model.AdminCartDeliveryPoint{}
	if r.OfferCheckout == nil {
		return out, nil
	}
	search2 := ""
	if search != nil {
		search2 = *search
	}
	pts, err := r.OfferCheckout.OfferDeliveryPoints(ctx, ch, c, groupKey, oid, search2)
	if err != nil {
		return nil, err
	}
	for _, p := range pts {
		out = append(out, &model.AdminCartDeliveryPoint{ID: p.ID, Name: p.Name, Address: formatPointAddress(p.Address)})
	}
	return out, nil
}
