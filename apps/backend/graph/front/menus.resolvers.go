package front

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/internal/cache"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/menu"
)

// Gated on a resolved channel AND assignment to it - unassigned menus are drafts, hidden from every storefront; any miss returns null so navigation degrades silently instead of erroring the page
// Items come back hierarchical (`MenuItem.children` carries the sub-tree) and every entity-target item's `url` is pre-resolved from url_rewrites, so the storefront renders without reshaping or knowing the target kind
func (r *queryResolver) Menu(ctx context.Context, slug string) (*model.Menu, error) {
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil, nil
	}
	locale := r.activeLocale(ctx)
	cacheKey := slug + ":" + sv.ID.String() + ":" + locale
	menuCache := r.MenuSvc.Cache()
	if hit, ok := cache.GetJSON[*model.Menu](ctx, menuCache, cacheKey); ok { // gqlgen re-serializes on the response anyway, so a JSON round-trip keeps everything the schema projects
		return hit, nil
	}
	m, err := r.MenuSvc.GetMenuBySlugForChannel(ctx, slug, sv.ID)
	if err != nil {
		if errors.Is(err, menu.ErrMenuNotFound) {
			menuCache.SetJSON(ctx, cacheKey, (*model.Menu)(nil), menuCacheTTL) // negative-cache the miss - keeps bot traffic on non-existent slugs off Postgres
			return nil, nil
		}
		return nil, err
	}
	items, err := r.MenuSvc.ListMenuItems(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	itemIDs := make([]uuid.UUID, len(items))
	for i, it := range items {
		itemIDs[i] = it.ID
	}
	translations, err := r.MenuSvc.ListMenuItemTranslationsByItemIDs(ctx, itemIDs)
	if err != nil {
		return nil, err
	}
	resolved := make([]menu.MenuItem, len(items))
	for i, it := range items {
		it.Label = menu.ResolveItemLabel(it, translations[it.ID], locale) // storefront sees a flat string regardless of how the override is stored
		resolved[i] = it
	}
	urls, err := resolveMenuItemURLs(ctx, r.URLRewriteSvc, resolved)
	if err != nil {
		return nil, err
	}
	out := toFrontMenuModel(m, resolved, urls)
	menuCache.SetJSON(ctx, cacheKey, out, menuCacheTTL)
	return out, nil
}

func (r *queryResolver) Menus(ctx context.Context, slugs []string) ([]*model.Menu, error) {
	// Misses are omitted rather than nil-padded - the storefront maps the result back by slug
	out := make([]*model.Menu, 0, len(slugs))
	for _, slug := range slugs {
		m, err := r.Menu(ctx, slug) // memoized per (slug, channel, locale), so repeated slugs cost nothing
		if err != nil {
			return nil, err
		}
		if m != nil {
			out = append(out, m)
		}
	}
	return out, nil
}
