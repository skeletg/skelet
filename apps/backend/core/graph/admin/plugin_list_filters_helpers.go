package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The list a plugin filter narrows and the permission that reads it
func pluginListKind(list model.PluginList) (sdk.ListKind, string) {
	switch list {
	case model.PluginListCustomers:
		return sdk.ListCustomers, "customers.view"
	case model.PluginListOrders:
		return sdk.ListOrders, "orders.view"
	default:
		return sdk.ListProducts, "catalog.view"
	}
}

// The ids the plugin filters picked on a list narrow it to - restrict is false when none was picked
func pluginFilterIDs(ctx context.Context, list model.PluginList, in []*model.PluginListFilterInput) (ids []uuid.UUID, restrict bool, err error) {
	if len(in) == 0 {
		return nil, false, nil
	}
	picks := make([]plugin.ListFilterPick, 0, len(in))
	for _, f := range in {
		picks = append(picks, plugin.ListFilterPick{Key: f.Key, Values: f.Values})
	}
	kind, _ := pluginListKind(list)
	return plugin.ListFilterIDs(ctx, kind, picks)
}
