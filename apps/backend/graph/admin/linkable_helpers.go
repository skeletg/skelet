package admin

// Lives here rather than *.resolvers.go - gqlgen relocates or comments out free functions there on regen

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
)

// One batched label lookup per entity type, falling back to request_path when nothing names the row - the input's path-sorted order survives
func (r *Resolver) buildLinkables(ctx context.Context, rows []urlrewrite.UrlRewrite) []*model.Linkable {
	locale := r.activeLocale(ctx)

	idsByType := map[string][]uuid.UUID{}
	for _, rw := range rows {
		et := string(rw.EntityType)
		idsByType[et] = append(idsByType[et], rw.EntityID)
	}

	labels := map[string]map[uuid.UUID]string{} // [entityType][entityId] = display name, only where resolvable
	for et, ids := range idsByType {
		switch et {
		case "product", "category", "collection":
			labels[et] = r.CatalogSvc.LinkLabelsByIDs(ctx, et, ids, locale)
		case "blog_post", "blog_category":
			labels[et] = r.BlogSvc.LinkLabelsByIDs(ctx, et, ids, locale)
		case "cms_page":
			labels[et] = r.CMSSvc.PageTitlesByIDs(ctx, ids, locale)
		}
	}

	out := make([]*model.Linkable, 0, len(rows))
	for _, rw := range rows {
		label := rw.RequestPath
		if m := labels[string(rw.EntityType)]; m != nil {
			if name := m[rw.EntityID]; name != "" {
				label = name
			}
		}
		out = append(out, &model.Linkable{
			EntityType: toUrlEntityType(rw.EntityType),
			EntityID:   rw.EntityID.String(),
			Label:      label,
			Path:       rw.RequestPath,
		})
	}
	return out
}
