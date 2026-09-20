package front

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func (r *mutationResolver) AddToWishlist(ctx context.Context, productID string) (bool, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return false, err
	}
	pid, err := uuid.Parse(productID)
	if err != nil {
		return false, invalidID("product")
	}
	if err := r.WishlistSvc.Add(ctx, claims.CustomerID, pid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) RemoveFromWishlist(ctx context.Context, productID string) (bool, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return false, err
	}
	pid, err := uuid.Parse(productID)
	if err != nil {
		return false, invalidID("product")
	}
	if err := r.WishlistSvc.Remove(ctx, claims.CustomerID, pid); err != nil {
		return false, err
	}
	return true, nil
}

// The cart add re-prices server-side and enforces the SIMPLE-only + stock guards, so anything needing options fails right here and the storefront sends those to the PDP instead
// Removal from the wishlist is best-effort - once the item is in the cart a failed removal mustn't fail the move
func (r *mutationResolver) MoveToCart(ctx context.Context, productID string) (*model1.Cart, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	pid, err := uuid.Parse(productID)
	if err != nil {
		return nil, invalidID("product")
	}
	store, ok := channel.FromContext(ctx)
	if !ok || store == nil {
		return nil, storeRequired()
	}
	prod, err := r.CatalogSvc.GetProduct(ctx, pid, r.activeLocale(ctx))
	if err != nil {
		return nil, apperr.New("PRODUCT_NOT_FOUND", apperr.ClassNotFound, "product not found")
	}
	name := prod.SKU
	if prod.Content != nil && prod.Content.Name != "" {
		name = prod.Content.Name
	}
	c, err := r.CartSvc.GetOrCreateCart(ctx, claims.CustomerID, store.ID)
	if err != nil {
		return nil, err
	}
	updated, err := r.CartSvc.AddToCart(ctx, c.ID, &claims.CustomerID, cart.AddCartLineInput{
		ProductSku:  prod.SKU,
		ProductName: name,
		UnitPrice:   0, // re-resolved server-side
		Quantity:    1,
	})
	if err != nil {
		return nil, err
	}
	_ = r.WishlistSvc.Remove(ctx, claims.CustomerID, pid)
	return r.fullCartModel(ctx, updated), nil
}

func (r *queryResolver) WishlistProductIds(ctx context.Context) ([]string, error) {
	cid := channel.OptionalCustomerID(ctx)
	if cid == nil {
		return []string{}, nil
	}
	ids, err := r.WishlistSvc.ListIDs(ctx, *cid)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out, nil
}

func (r *queryResolver) MyWishlist(ctx context.Context) ([]*model.Product, error) {
	cid := channel.OptionalCustomerID(ctx)
	if cid == nil {
		return []*model.Product{}, nil
	}
	ids, err := r.WishlistSvc.ListIDs(ctx, *cid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Product, 0, len(ids))
	var visible map[uuid.UUID]struct{} // drops items not sold on the active store while keeping wishlist order, newest first
	if sid := channelIDFromContext(ctx); sid != nil {
		if pub, perr := r.CatalogSvc.FilterVisibleProductIDsForChannel(ctx, ids, *sid); perr == nil {
			visible = pub
		}
	}
	for _, id := range ids {
		if visible != nil {
			if _, ok := visible[id]; !ok {
				continue
			}
		}
		p, perr := r.CatalogSvc.GetProduct(ctx, id, r.activeLocale(ctx))
		if perr != nil {
			continue
		}
		m, rerr := r.renderProduct(ctx, p)
		if rerr != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *queryResolver) MyWishlistShareToken(ctx context.Context) (string, error) {
	// minted once, then returned on every later call
	cid := channel.OptionalCustomerID(ctx)
	if cid == nil {
		return "", apperr.New("UNAUTHENTICATED", apperr.ClassAuth, "sign in to share your wishlist")
	}
	return r.WishlistSvc.ShareToken(ctx, *cid)
}

// Public read-only view behind a share token - channel-visible products only, unknown tokens give an empty list, and it's rate-limited because tokens are unguessable but scanning shouldn't be free
func (r *queryResolver) SharedWishlist(ctx context.Context, token string) ([]*model.Product, error) {
	if err := r.checkRate(ctx, "shared_wishlist"); err != nil {
		return nil, err
	}
	ids, err := r.WishlistSvc.ResolveShare(ctx, token)
	if err != nil || len(ids) == 0 {
		return []*model.Product{}, nil
	}
	out := make([]*model.Product, 0, len(ids))
	var visible map[uuid.UUID]struct{}
	if sid := channelIDFromContext(ctx); sid != nil {
		if pub, perr := r.CatalogSvc.FilterVisibleProductIDsForChannel(ctx, ids, *sid); perr == nil {
			visible = pub
		}
	}
	for _, id := range ids {
		if visible != nil {
			if _, ok := visible[id]; !ok {
				continue
			}
		}
		p, perr := r.CatalogSvc.GetProduct(ctx, id, r.activeLocale(ctx))
		if perr != nil {
			continue
		}
		m, rerr := r.renderProduct(ctx, p)
		if rerr != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}
