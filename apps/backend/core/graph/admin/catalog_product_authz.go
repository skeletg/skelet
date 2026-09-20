package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// requireCatalogManageOnAllListingChannels gates product-global writes (attribute values, media, structure, downloads, terms) the way UpsertProductContent gates the shared content rows: the row renders on every channel listing the product, so a channel-scoped catalog.manage must cover them all, or the edit leaks into channels the admin can't see
// The empty locale means the edit isn't locale-bound, so per channel a grant of any locale scope counts; an unlisted product falls back to the gate's bare slug check, having no leak surface
func (r *mutationResolver) requireCatalogManageOnAllListingChannels(ctx context.Context, productID uuid.UUID) error {
	// The cheap any-scope check first, so anonymous and grant-less callers are refused before the listings query below does any work
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return err
	}
	listings, err := r.CatalogSvc.ListProductChannelListings(ctx, productID)
	if err != nil {
		return err
	}
	channelIDs := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		channelIDs[i] = l.ChannelID
	}
	return auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, "catalog.manage", channelIDs, "")
}

// The same product-global gate, reached through a child row - an option, a value, a bundle option or selection, a grouped member, a kit component
// since each renders on every channel listing its OWNING product, and the caller only has the child's id

func (r *mutationResolver) requireCatalogManageForProductOption(ctx context.Context, optionID uuid.UUID) error {
	opt, err := r.CatalogSvc.GetProductOption(ctx, optionID)
	if err != nil {
		return err
	}
	return r.requireCatalogManageOnAllListingChannels(ctx, opt.ProductID)
}

func (r *mutationResolver) requireCatalogManageForProductOptionValue(ctx context.Context, valueID uuid.UUID) error {
	val, err := r.CatalogSvc.GetProductOptionValue(ctx, valueID)
	if err != nil {
		return err
	}
	return r.requireCatalogManageForProductOption(ctx, val.OptionID)
}

func (r *mutationResolver) requireCatalogManageForBundleOption(ctx context.Context, optionID uuid.UUID) error {
	opt, err := r.CatalogSvc.GetBundleOption(ctx, optionID)
	if err != nil {
		return err
	}
	return r.requireCatalogManageOnAllListingChannels(ctx, opt.ProductID)
}

// Through the OPTION, not the selection's own ProductID - that one is the product being selected,
// while the structure being edited belongs to the bundle
func (r *mutationResolver) requireCatalogManageForBundleSelection(ctx context.Context, selectionID uuid.UUID) error {
	sel, err := r.CatalogSvc.GetBundleSelection(ctx, selectionID)
	if err != nil {
		return err
	}
	return r.requireCatalogManageForBundleOption(ctx, sel.OptionID)
}

func (r *mutationResolver) requireCatalogManageForGroupedMember(ctx context.Context, memberID uuid.UUID) error {
	member, err := r.CatalogSvc.GetGroupedMember(ctx, memberID)
	if err != nil {
		return err
	}
	return r.requireCatalogManageOnAllListingChannels(ctx, member.ParentID)
}

func (r *mutationResolver) requireCatalogManageForKitComponent(ctx context.Context, componentID uuid.UUID) error {
	comp, err := r.CatalogSvc.GetKitComponent(ctx, componentID)
	if err != nil {
		return err
	}
	return r.requireCatalogManageOnAllListingChannels(ctx, comp.ProductID)
}
