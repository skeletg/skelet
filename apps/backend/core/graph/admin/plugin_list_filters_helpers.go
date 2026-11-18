package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The list a plugin filter narrows and the permission that reads it. A list added to the schema and not here is an
// error, not the products list under the catalog's permission
func pluginListKind(list model.PluginList) (sdk.ListKind, string, error) {
	switch list {
	case model.PluginListProducts:
		return sdk.ListProducts, "catalog.view", nil
	case model.PluginListCustomers:
		return sdk.ListCustomers, "customers.view", nil
	case model.PluginListOrders:
		return sdk.ListOrders, "orders.view", nil
	}
	return "", "", fmt.Errorf("plugin list %q has no filters wired", list)
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
	kind, _, err := pluginListKind(list)
	if err != nil {
		return nil, false, err
	}
	return plugin.ListFilterIDs(ctx, kind, picks)
}
