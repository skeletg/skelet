package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateMenu(ctx context.Context, input model.CreateMenuInput) (*model.Menu, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	m, err := r.MenuSvc.CreateMenu(ctx, menu.CreateMenuInput{Slug: input.Slug, Name: input.Name})
	if err != nil {
		return nil, err
	}
	return toMenuModel(m, nil), nil
}

func (r *mutationResolver) UpdateMenu(ctx context.Context, input model.UpdateMenuInput) (*model.Menu, error) {
	id, err := parseID(input.ID, "menu")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnMenuChannels(ctx, id); err != nil {
		return nil, err
	}
	m, err := r.MenuSvc.UpdateMenu(ctx, menu.UpdateMenuInput{
		ID:   id,
		Slug: input.Slug,
		Name: input.Name,
	})
	if err != nil {
		return nil, err
	}
	return toMenuModel(m, nil), nil
}

func (r *mutationResolver) DeleteMenu(ctx context.Context, id string) (bool, error) {
	mid, err := parseID(id, "menu")
	if err != nil {
		return false, err
	}
	if err := r.requireOnMenuChannels(ctx, mid); err != nil {
		return false, err
	}
	if err := r.MenuSvc.DeleteMenu(ctx, mid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetMenuStores(ctx context.Context, menuID string, channelIds []string) (*model.Menu, error) {
	// an empty list converts the menu back to a draft
	mid, err := parseID(menuID, "menu")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnMenuChannels(ctx, mid); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(channelIds))
	for _, s := range channelIds {
		id, err := parseID(s, "channel")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	// Gating the current set above keeps a removal from stripping a channel the caller can't manage
	for _, id := range ids {
		if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "cms.manage", id, nil); err != nil {
			return nil, err
		}
	}
	if err := r.MenuSvc.SetMenuStores(ctx, mid, ids); err != nil {
		return nil, err
	}
	return r.hydratedAdminMenu(ctx, mid)
}

func (r *mutationResolver) CreateMenuItem(ctx context.Context, input model.CreateMenuItemInput) (*model.MenuItem, error) {
	menuID, err := parseID(input.MenuID, "menu")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnMenuChannels(ctx, menuID); err != nil {
		return nil, err
	}
	parentID, err := parseOptionalUUIDStrict(input.ParentID)
	if err != nil {
		return nil, errInvalidID("parent id", nil)
	}
	targetID, err := parseOptionalUUIDStrict(input.TargetID)
	if err != nil {
		return nil, errInvalidID("target id", nil)
	}
	url := ""
	if input.URL != nil {
		url = *input.URL
	}
	sortOrder := int32(0)
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}
	item, err := r.MenuSvc.CreateMenuItem(ctx, menu.CreateMenuItemInput{
		MenuID:     menuID,
		ParentID:   parentID,
		Label:      input.Label,
		TargetType: menuTargetTypeFromModel(input.TargetType),
		TargetID:   targetID,
		URL:        url,
		SortOrder:  sortOrder,
	})
	if err != nil {
		return nil, err
	}
	return toMenuItemModel(item), nil
}

func (r *mutationResolver) UpdateMenuItem(ctx context.Context, input model.UpdateMenuItemInput) (*model.MenuItem, error) {
	id, err := parseID(input.ID, "menu item")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnMenuItemChannels(ctx, id); err != nil {
		return nil, err
	}
	parentID, err := parseOptionalUUIDStrict(input.ParentID)
	if err != nil {
		return nil, errInvalidID("parent id", nil)
	}
	targetID, err := parseOptionalUUIDStrict(input.TargetID)
	if err != nil {
		return nil, errInvalidID("target id", nil)
	}
	in := menu.UpdateMenuItemInput{
		ID:        id,
		Label:     input.Label,
		ParentID:  parentID,
		URL:       input.URL,
		SortOrder: input.SortOrder,
	}
	if input.ClearParent != nil && *input.ClearParent {
		in.ClearParent = true
		in.ParentID = nil
	}
	if input.TargetType != nil {
		tt := menuTargetTypeFromModel(input.TargetType)
		in.TargetType = &tt
	}
	if targetID != nil {
		in.TargetID = targetID
	}
	item, err := r.MenuSvc.UpdateMenuItem(ctx, in)
	if err != nil {
		return nil, err
	}
	return toMenuItemModel(item), nil
}

func (r *mutationResolver) DeleteMenuItem(ctx context.Context, id string) (bool, error) {
	itemID, err := parseID(id, "menu item")
	if err != nil {
		return false, err
	}
	if err := r.requireOnMenuItemChannels(ctx, itemID); err != nil {
		return false, err
	}
	if err := r.MenuSvc.DeleteMenuItem(ctx, itemID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) ReorderMenuItems(ctx context.Context, menuID string, items []*model.ReorderMenuItemInput) (*model.Menu, error) {
	mid, err := parseID(menuID, "menu")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnMenuChannels(ctx, mid); err != nil {
		return nil, err
	}
	updates := make([]menu.ReorderMenuItem, 0, len(items))
	for _, it := range items {
		id, err := parseID(it.ID, "menu item")
		if err != nil {
			return nil, err
		}
		parentID, err := parseOptionalUUIDStrict(it.ParentID)
		if err != nil {
			return nil, errInvalidID("parent id", nil)
		}
		updates = append(updates, menu.ReorderMenuItem{
			ID:        id,
			ParentID:  parentID,
			SortOrder: it.SortOrder,
		})
	}
	if err := r.MenuSvc.ReorderMenuItems(ctx, mid, updates); err != nil {
		return nil, err
	}
	return r.hydratedAdminMenu(ctx, mid)
}

func (r *mutationResolver) UpsertMenuItemTranslation(ctx context.Context, itemID string, locale string, label string) (*model.MenuItemTranslation, error) {
	id, err := parseID(itemID, "menu item")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnMenuItemChannels(ctx, id); err != nil {
		return nil, err
	}
	t, err := r.MenuSvc.UpsertMenuItemTranslation(ctx, id, locale, label)
	if err != nil {
		return nil, err
	}
	return toMenuItemTranslationModel(t), nil
}

func (r *mutationResolver) DeleteMenuItemTranslation(ctx context.Context, itemID string, locale string) (bool, error) {
	id, err := parseID(itemID, "menu item")
	if err != nil {
		return false, err
	}
	if err := r.requireOnMenuItemChannels(ctx, id); err != nil {
		return false, err
	}
	if err := r.MenuSvc.DeleteMenuItemTranslation(ctx, id, locale); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) Menus(ctx context.Context) ([]*model.Menu, error) {
	// carries store assignments so admins see where a menu lives without opening the detail page
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	menus, err := r.MenuSvc.ListMenus(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(menus))
	for i, m := range menus {
		ids[i] = m.ID
	}
	assignments, err := r.MenuSvc.ListMenuChannelAssignmentsByMenuIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Menu, len(menus))
	for i, m := range menus {
		m.ChannelIDs = assignments[m.ID]
		out[i] = toMenuModel(m, nil) // items and translations load per-menu in the detail page - the list only renders slug, name, itemCount and store assignments
	}
	return out, nil
}

func (r *queryResolver) Menu(ctx context.Context, id string) (*model.Menu, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	mid, err := parseID(id, "menu")
	if err != nil {
		return nil, err
	}
	return r.hydratedAdminMenu(ctx, mid)
}

func (r *queryResolver) MenuBySlug(ctx context.Context, slug string) (*model.Menu, error) {
	// same hydration as Menu(id)
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	m, err := r.MenuSvc.GetMenuBySlug(ctx, slug)
	if err != nil {
		return nil, nil
	}
	return r.hydratedAdminMenu(ctx, m.ID)
}
