package admin

import (
	"context"
	"time"

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

func (r *mutationResolver) AdminAddCartLine(ctx context.Context, cartID string, productSku string, quantity float64, options []*model.AdminCartLineOptionInput) (*model.AdminCart, error) {
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
		c, err := r.CartSvc.AdminAddConfiguredToCart(ctx, id, cart.AddConfiguredToCartInput{ProductSku: productSku, Quantity: quantity, Options: picks})
		if err != nil {
			return nil, err
		}
		return r.mapCart(ctx, c), nil
	}
	c, err := r.CartSvc.AdminAddToCart(ctx, id, cart.AddCartLineInput{ProductSku: productSku, Quantity: quantity})
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

func (r *queryResolver) Carts(ctx context.Context, channelID string, customerID *string, status *string, abandoned *bool, search *string, limit *int32, offset *int32) (*model.CartList, error) {
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
		Search:     search,
		Limit:      50,
		Offset:     0,
	}
	if limit != nil && *limit > 0 {
		f.Limit = *limit
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
