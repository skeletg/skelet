package admin

// Lives here rather than menus.resolvers.go - gqlgen's regen pass moves free functions into a comment block

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
)

// Full hydration so the mutations returning a freshly modified menu need no follow-up refetch
func (r *Resolver) hydratedAdminMenu(ctx context.Context, menuID uuid.UUID) (*model.Menu, error) {
	m, err := r.MenuSvc.GetMenuByID(ctx, menuID)
	if err != nil {
		return nil, err
	}
	items, err := r.MenuSvc.ListMenuItems(ctx, menuID)
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
	for i := range items {
		items[i].Translations = translations[items[i].ID]
	}
	stores, err := r.MenuSvc.ListMenuChannelAssignments(ctx, menuID)
	if err != nil {
		return nil, err
	}
	m.ChannelIDs = stores
	return toMenuModel(m, items), nil
}

func toMenuModel(m menu.Menu, items []menu.MenuItem) *model.Menu {
	itemModels := make([]*model.MenuItem, len(items))
	for i, it := range items {
		itemModels[i] = toMenuItemModel(it)
	}
	storeIDs := make([]string, len(m.ChannelIDs))
	for i, id := range m.ChannelIDs {
		storeIDs[i] = id.String()
	}
	return &model.Menu{
		ID:         m.ID.String(),
		Slug:       m.Slug,
		Name:       m.Name,
		ItemCount:  int32(m.ItemCount),
		Items:      itemModels,
		ChannelIds: storeIDs,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

func toMenuItemModel(it menu.MenuItem) *model.MenuItem {
	out := &model.MenuItem{
		ID:        it.ID.String(),
		MenuID:    it.MenuID.String(),
		Label:     it.Label,
		SortOrder: it.SortOrder,
		CreatedAt: it.CreatedAt,
		UpdatedAt: it.UpdatedAt,
	}
	if it.ParentID != nil {
		v := it.ParentID.String()
		out.ParentID = &v
	}
	if it.TargetType != "" {
		tt := menuTargetTypeToModel(it.TargetType)
		out.TargetType = &tt
	}
	if it.TargetID != nil {
		v := it.TargetID.String()
		out.TargetID = &v
	}
	if it.URL != "" {
		u := it.URL
		out.URL = &u
	}
	out.Translations = make([]*model.MenuItemTranslation, len(it.Translations))
	for i, t := range it.Translations {
		out.Translations[i] = toMenuItemTranslationModel(t)
	}
	return out
}

func toMenuItemTranslationModel(t menu.MenuItemTranslation) *model.MenuItemTranslation {
	return &model.MenuItemTranslation{
		Locale:    t.Locale,
		Label:     t.Label,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// The domain's lowercase target_type vocabulary matches url_rewrites.entity_type, the GraphQL enum is SCREAMING_SNAKE
func menuTargetTypeToModel(t menu.TargetType) model.MenuItemTargetType {
	switch t {
	case menu.TargetTypeProduct:
		return model.MenuItemTargetTypeProduct
	case menu.TargetTypeCategory:
		return model.MenuItemTargetTypeCategory
	case menu.TargetTypeCollection:
		return model.MenuItemTargetTypeCollection
	case menu.TargetTypeBlogPost:
		return model.MenuItemTargetTypeBlogPost
	case menu.TargetTypeBlogCategory:
		return model.MenuItemTargetTypeBlogCategory
	case menu.TargetTypeBlogTag:
		return model.MenuItemTargetTypeBlogTag
	case menu.TargetTypeBlogAuthor:
		return model.MenuItemTargetTypeBlogAuthor
	case menu.TargetTypeCMSPage:
		return model.MenuItemTargetTypeCmsPage
	}
	return model.MenuItemTargetTypeProduct
}

func menuTargetTypeFromModel(t *model.MenuItemTargetType) menu.TargetType {
	if t == nil {
		return ""
	}
	switch *t {
	case model.MenuItemTargetTypeProduct:
		return menu.TargetTypeProduct
	case model.MenuItemTargetTypeCategory:
		return menu.TargetTypeCategory
	case model.MenuItemTargetTypeCollection:
		return menu.TargetTypeCollection
	case model.MenuItemTargetTypeBlogPost:
		return menu.TargetTypeBlogPost
	case model.MenuItemTargetTypeBlogCategory:
		return menu.TargetTypeBlogCategory
	case model.MenuItemTargetTypeBlogTag:
		return menu.TargetTypeBlogTag
	case model.MenuItemTargetTypeBlogAuthor:
		return menu.TargetTypeBlogAuthor
	case model.MenuItemTargetTypeCmsPage:
		return menu.TargetTypeCMSPage
	}
	return ""
}

func parseOptionalUUIDStrict(s *string) (*uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	u, err := uuid.Parse(*s)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
