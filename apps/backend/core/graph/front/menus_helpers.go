package front // menus.resolvers.go helpers live here so gqlgen's regeneration pass doesn't shove them into a quarantine block at the bottom

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
)

const menuCacheTTL = 5 * time.Minute // dashboard menu edits reach shoppers within minutes, and the per-store menu still rarely re-renders from Postgres

func toFrontMenuModel(m menu.Menu, items []menu.MenuItem, resolved map[uuid.UUID]string) *model.Menu { // `resolved` is keyed by item ID - pass an empty map when nothing needs URL resolution
	nodes := make(map[uuid.UUID]*model.MenuItem, len(items)) // every node exists before the second pass wires parent → child, so input order doesn't matter
	for _, it := range items {
		nodes[it.ID] = toFrontMenuItemModel(it, resolved[it.ID])
	}
	roots := make([]*model.MenuItem, 0, len(items))
	for _, it := range items {
		node := nodes[it.ID]
		if it.ParentID != nil {
			if parent, ok := nodes[*it.ParentID]; ok {
				parent.Children = append(parent.Children, node)
				continue
			}
			// Stale parent_id → surface the orphan at top level rather than dropping the row
		}
		roots = append(roots, node)
	}
	sortMenuItemTree(roots) // the repo already orders rows - re-sorting keeps this converter from silently depending on the caller
	return &model.Menu{
		ID:    m.ID.String(),
		Slug:  m.Slug,
		Name:  m.Name,
		Items: roots,
	}
}

func sortMenuItemTree(items []*model.MenuItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].ID < items[j].ID
	})
	for _, it := range items {
		if len(it.Children) > 0 {
			sortMenuItemTree(it.Children)
		}
	}
}

func toFrontMenuItemModel(it menu.MenuItem, resolvedURL string) *model.MenuItem {
	mi := &model.MenuItem{
		ID:        it.ID.String(),
		Label:     it.Label,
		SortOrder: it.SortOrder,
		Children:  []*model.MenuItem{},
	}
	if it.ParentID != nil {
		v := it.ParentID.String()
		mi.ParentID = &v
	}
	if it.TargetType != "" {
		tt := frontMenuTargetTypeToModel(it.TargetType)
		mi.TargetType = &tt
	}
	if it.TargetID != nil {
		v := it.TargetID.String()
		mi.TargetID = &v
	}
	if it.URL != "" { // external URL wins over the resolved canonical path, and URL stays nil when neither is set
		u := it.URL
		mi.URL = &u
	} else if resolvedURL != "" {
		u := resolvedURL
		mi.URL = &u
	}
	return mi
}

func menuTargetToURLRewriteEntity(t menu.TargetType) urlrewrite.EntityType {
	switch t {
	case menu.TargetTypeProduct:
		return urlrewrite.EntityTypeProduct
	case menu.TargetTypeCategory:
		return urlrewrite.EntityTypeCategory
	case menu.TargetTypeCollection:
		return urlrewrite.EntityTypeCollection
	case menu.TargetTypeBlogPost:
		return urlrewrite.EntityTypeBlogPost
	case menu.TargetTypeBlogCategory:
		return urlrewrite.EntityTypeBlogCategory
	case menu.TargetTypeBlogTag:
		return urlrewrite.EntityTypeBlogTag
	case menu.TargetTypeBlogAuthor:
		return urlrewrite.EntityTypeBlogAuthor
	case menu.TargetTypeCMSPage:
		return urlrewrite.EntityTypeCMSPage
	}
	return ""
}

func frontMenuTargetTypeToModel(t menu.TargetType) model.MenuItemTargetType {
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

func resolveMenuItemURLs(
	ctx context.Context,
	svc *urlrewrite.Service,
	items []menu.MenuItem,
) (map[uuid.UUID]string, error) { // keyed by item ID, not entity ID - one batched url_rewrites lookup per distinct entity type
	out := make(map[uuid.UUID]string, len(items))
	if svc == nil {
		return out, nil
	}
	byType := make(map[urlrewrite.EntityType][]uuid.UUID)
	idToItem := make(map[uuid.UUID]uuid.UUID, len(items)) // entity_id → item_id
	for _, it := range items {
		if it.TargetType == "" || it.TargetID == nil {
			continue
		}
		et := menuTargetToURLRewriteEntity(it.TargetType)
		if et == "" {
			continue
		}
		byType[et] = append(byType[et], *it.TargetID)
		idToItem[*it.TargetID] = it.ID
	}
	for et, ids := range byType {
		rewrites, err := svc.ListByEntityIDs(ctx, et, ids)
		if err != nil {
			return nil, err
		}
		for _, rw := range rewrites {
			if rw.RedirectType != urlrewrite.RedirectTypeCanonical {
				continue
			}
			itemID, ok := idToItem[rw.EntityID]
			if !ok {
				continue
			}
			out[itemID] = "/" + rw.RequestPath // canonical rows are stored without a leading slash - the storefront's <Link href> needs one to resolve at the root
		}
	}
	return out, nil
}
