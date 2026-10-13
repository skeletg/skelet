package admin

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// A cart or order on another channel would hand the customer's identity to it
func (r *Resolver) requireCustomerOnChannel(ctx context.Context, customerID *uuid.UUID, channelID uuid.UUID) error {
	if customerID == nil {
		return nil
	}
	c, err := r.CustomerSvc.GetByID(ctx, *customerID)
	if err != nil {
		return errNotFound("customer")
	}
	if c.ChannelID != channelID {
		return apperr.New("CUSTOMER_WRONG_CHANNEL", apperr.ClassValidation, "that customer belongs to another channel")
	}
	return nil
}

// The lookup-first gate every cart mutation shares - the cart has to be loaded before orders.update can be scoped to its channel
func (r *mutationResolver) cartForAdmin(ctx context.Context, idStr string) (uuid.UUID, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "orders.update"); err != nil {
		return uuid.Nil, err
	}
	cartID, err := parseID(idStr, "cart")
	if err != nil {
		return uuid.Nil, err
	}
	c, err := r.CartSvc.Repo().GetCartByID(ctx, cartID)
	if err != nil {
		return uuid.Nil, errNotFound("cart")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", c.ChannelID, nil); err != nil {
		return uuid.Nil, err
	}
	return cartID, nil
}

const cartAbandonedAfter = time.Hour // how long an active cart sits untouched before the `abandoned` filter surfaces it

const maxCartListLimit = 200

// Carts store no totals, so line totals and the subtotal are computed here
func toAdminCartModel(c cart.Cart, email, name *string, currency string) *model.AdminCart {
	lines := make([]*model.AdminCartLine, len(c.Lines))
	var subtotal int64
	for i, l := range c.Lines {
		lt := money.LineTotal(l.UnitPrice, l.Quantity)
		subtotal += lt
		comps := make([]*model.AdminCartLineComponent, len(l.Components))
		for j, cc := range l.Components {
			comps[j] = &model.AdminCartLineComponent{
				ID:        cc.ID.String(),
				Sku:       cc.ProductSku,
				Name:      cc.ProductName,
				UnitPrice: int32(cc.UnitPrice),
				Quantity:  cc.Quantity,
			}
		}
		opts := make([]*model.AdminCartLineOption, len(l.Options))
		for j, o := range l.Options {
			opts[j] = &model.AdminCartLineOption{
				ID:          o.ID.String(),
				OptionTitle: o.OptionTitle,
				ValueLabel:  o.ValueLabel,
				TextValue:   o.TextValue,
				PriceDelta:  int32(o.PriceDelta),
				FileURL:     o.FileURL,
			}
		}
		lines[i] = &model.AdminCartLine{
			ID:         l.ID.String(),
			Name:       l.ProductName,
			Sku:        l.ProductSku,
			UnitPrice:  int32(l.UnitPrice),
			Quantity:   l.Quantity,
			TotalPrice: int32(lt),
			IsSimple:   len(l.Components) == 0 && len(l.Options) == 0,
			Components: comps,
			Options:    opts,
		}
	}
	m := &model.AdminCart{
		ID:            c.ID.String(),
		CustomerEmail: email,
		CustomerName:  name,
		ChannelID:     c.ChannelID.String(),
		CurrencyCode:  currency,
		Status:        c.Status,
		Lines:         lines,
		Subtotal:      int32(subtotal),
		ItemCount:     int32(len(lines)),
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
	if c.CustomerID != nil {
		s := c.CustomerID.String()
		m.CustomerID = &s
	}
	return m
}

func toCartListItemModel(it cart.CartListItem, currency string) *model.CartListItem {
	m := &model.CartListItem{
		ID:            it.ID.String(),
		CustomerEmail: it.CustomerEmail,
		ChannelID:     it.ChannelID.String(),
		CurrencyCode:  currency,
		Status:        it.Status,
		ItemCount:     it.ItemCount,
		Subtotal:      int32(it.Subtotal),
		UpdatedAt:     it.UpdatedAt,
	}
	if it.CustomerID != nil {
		s := it.CustomerID.String()
		m.CustomerID = &s
	}
	if n := joinName(it.CustomerFirstName, it.CustomerLastName); n != "" {
		m.CustomerName = &n
	}
	return m
}

func joinName(first, last *string) string {
	f, l := "", ""
	if first != nil {
		f = *first
	}
	if last != nil {
		l = *last
	}
	return strings.TrimSpace(f + " " + l)
}

// (nil, nil) for a guest cart and for a failed lookup alike - the admin view degrades rather than erroring
func (r *Resolver) cartCustomerInfo(ctx context.Context, customerID *uuid.UUID) (*string, *string) {
	if customerID == nil || r.CustomerSvc == nil {
		return nil, nil
	}
	c, err := r.CustomerSvc.GetByIDWithAddresses(ctx, *customerID)
	if err != nil {
		return nil, nil
	}
	email := c.Email
	var namePtr *string
	if n := strings.TrimSpace(c.FirstName + " " + c.LastName); n != "" {
		namePtr = &n
	}
	return &email, namePtr
}

// Carts store no currency of their own, it's the channel default - USD when even that's unavailable
func (r *Resolver) channelCurrency(ctx context.Context, channelID uuid.UUID) string {
	if r.ChannelSvc == nil {
		return "USD"
	}
	ch, err := r.ChannelSvc.GetByID(ctx, channelID)
	if err != nil || ch.CurrencyCode == "" {
		return "USD"
	}
	return ch.CurrencyCode
}

// Expects the lines to be loaded already
func (r *Resolver) mapCart(ctx context.Context, c cart.Cart) *model.AdminCart {
	email, name := r.cartCustomerInfo(ctx, c.CustomerID)
	return toAdminCartModel(c, email, name, r.channelCurrency(ctx, c.ChannelID))
}
