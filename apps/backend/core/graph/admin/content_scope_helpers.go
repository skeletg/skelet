package admin

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Shared content renders on every channel that lists it, so writing it needs the grant on each of those channels
func (r *Resolver) requireOnPageChannels(ctx context.Context, slug string, pageID uuid.UUID, locale string) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return err
	}
	ids, err := r.pageListingChannels(ctx, pageID)
	if err != nil {
		return err
	}
	return auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, slug, ids, locale)
}

func (r *Resolver) pageListingChannels(ctx context.Context, pageID uuid.UUID) ([]uuid.UUID, error) {
	listings, err := r.CMSSvc.ListPageChannelListings(ctx, pageID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		ids[i] = l.ChannelID
	}
	return ids, nil
}

func (r *Resolver) postListingChannels(ctx context.Context, postID uuid.UUID) ([]uuid.UUID, error) {
	listings, err := r.BlogSvc.ListPostChannelListings(ctx, postID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		ids[i] = l.ChannelID
	}
	return ids, nil
}

func (r *Resolver) categoryListingChannels(ctx context.Context, categoryID uuid.UUID) ([]uuid.UUID, error) {
	listings, err := r.CatalogSvc.ListCategoryChannelListings(ctx, categoryID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		ids[i] = l.ChannelID
	}
	return ids, nil
}

// Adding or removing a listing changes who may edit the shared row, so it needs the target channel and every channel already listing it
func requireListingChange(ctx context.Context, checker auth.AllChannelsListedChecker, slug string, target uuid.UUID, listed []uuid.UUID, removing bool) error {
	if err := auth.RequireChannelPermission(ctx, checker, slug, target, nil); err != nil {
		return err
	}
	if !removing && slices.Contains(listed, target) {
		return nil
	}
	return auth.RequireAllListedChannelsPermission(ctx, checker, slug, listed, "")
}

func (r *Resolver) requirePageListingChange(ctx context.Context, pageID, target uuid.UUID, removing bool) error {
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "cms.manage", target, nil); err != nil {
		return err
	}
	ids, err := r.pageListingChannels(ctx, pageID)
	if err != nil {
		return err
	}
	return requireListingChange(ctx, r.AdminSvc, "cms.manage", target, ids, removing)
}

func (r *Resolver) requirePostListingChange(ctx context.Context, postID, target uuid.UUID, removing bool) error {
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "blog.manage", target, nil); err != nil {
		return err
	}
	ids, err := r.postListingChannels(ctx, postID)
	if err != nil {
		return err
	}
	return requireListingChange(ctx, r.AdminSvc, "blog.manage", target, ids, removing)
}

func (r *Resolver) requireCategoryListingChange(ctx context.Context, categoryID, target uuid.UUID, removing bool) error {
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "catalog.manage", target, nil); err != nil {
		return err
	}
	ids, err := r.categoryListingChannels(ctx, categoryID)
	if err != nil {
		return err
	}
	return requireListingChange(ctx, r.AdminSvc, "catalog.manage", target, ids, removing)
}

func (r *Resolver) requireOnPagesChannels(ctx context.Context, slug string, pageIDs []uuid.UUID) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return err // an empty id list still needs a caller who holds the permission somewhere
	}
	for _, id := range pageIDs {
		if err := r.requireOnPageChannels(ctx, slug, id, ""); err != nil {
			return err
		}
	}
	return nil
}

func (r *Resolver) requireOnPostChannels(ctx context.Context, slug string, postID uuid.UUID, locale string) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return err
	}
	ids, err := r.postListingChannels(ctx, postID)
	if err != nil {
		return err
	}
	return auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, slug, ids, locale)
}

func (r *Resolver) requireOnCategoryChannels(ctx context.Context, slug string, categoryID uuid.UUID) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return err
	}
	ids, err := r.categoryListingChannels(ctx, categoryID)
	if err != nil {
		return err
	}
	return auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, slug, ids, "")
}

func (r *Resolver) requireOnMenuChannels(ctx context.Context, menuID uuid.UUID) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return err
	}
	ids, err := r.MenuSvc.ListMenuChannelAssignments(ctx, menuID)
	if err != nil {
		return err
	}
	return auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, "cms.manage", ids, "")
}

func (r *Resolver) requireOnMenuItemChannels(ctx context.Context, itemID uuid.UUID) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return err
	}
	menuID, err := r.MenuSvc.MenuIDForItem(ctx, itemID)
	if err != nil {
		return err
	}
	return r.requireOnMenuChannels(ctx, menuID)
}

func (r *Resolver) requireGlobal(ctx context.Context, slug string) error {
	return auth.RequireGlobalPermission(ctx, r.AdminSvc, slug)
}

// Membership shows on the category's channels and on the product's, so both have to be in reach
func (r *mutationResolver) requireOnCategoryAndProductChannels(ctx context.Context, categoryID, productID uuid.UUID) error {
	if err := r.requireOnCategoryChannels(ctx, "catalog.manage", categoryID); err != nil {
		return err
	}
	return r.requireCatalogManageOnAllListingChannels(ctx, productID)
}

func (r *mutationResolver) requireOnProductChannelsInLocale(ctx context.Context, productID uuid.UUID, locale string) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return err
	}
	listings, err := r.CatalogSvc.ListProductChannelListings(ctx, productID)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		ids[i] = l.ChannelID
	}
	return auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, "catalog.manage", ids, locale)
}

// A channel's own slug needs that channel, and the shared one every channel listing the product
func (r *mutationResolver) requireProductSlugScope(ctx context.Context, productID uuid.UUID, channelID *uuid.UUID) error {
	if channelID != nil {
		return auth.RequireChannelPermission(ctx, r.AdminSvc, "catalog.manage", *channelID, nil)
	}
	return r.requireCatalogManageOnAllListingChannels(ctx, productID)
}

func (r *Resolver) requireOnProductChannels(ctx context.Context, slug string, productID uuid.UUID) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return err
	}
	listings, err := r.CatalogSvc.ListProductChannelListings(ctx, productID)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		ids[i] = l.ChannelID
	}
	return auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, slug, ids, "")
}

func (r *Resolver) visiblePageListings(ctx context.Context, pages ...*model.CMSPage) error {
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "cms.view")
	if err != nil {
		return err
	}
	if perm.Bypass() {
		return nil
	}
	for _, p := range pages {
		if p == nil {
			continue
		}
		kept := make([]*model.CMSPageChannelListing, 0, len(p.ChannelListings))
		for _, l := range p.ChannelListings {
			if cid, err := uuid.Parse(l.ChannelID); err == nil && perm.Allows(cid) {
				kept = append(kept, l)
			}
		}
		p.ChannelListings = kept
	}
	return nil
}
