package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *queryResolver) PluginListFilters(ctx context.Context, list model.PluginList) ([]*model.PluginListFilter, error) {
	kind, perm, err := pluginListKind(list)
	if err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, perm); err != nil {
		return nil, err
	}
	filters := plugin.ListFilters(ctx, kind)
	out := make([]*model.PluginListFilter, 0, len(filters))
	for _, f := range filters {
		opts := make([]*model.PluginListFilterOption, 0, len(f.Filter.Options))
		for _, o := range f.Filter.Options {
			opts = append(opts, &model.PluginListFilterOption{Value: o.Value, Label: o.Label})
		}
		out = append(out, &model.PluginListFilter{Key: f.Key(), Plugin: f.PluginSlug, Label: f.Filter.Label, Options: opts})
	}
	return out, nil
}
