package plugin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// OrderNumber asks the first numberer whose plugin is on for the channel, its counts kept apart under its slug
func OrderNumber(ctx context.Context, req order.NumberRequest) (num string, ok bool, err error) {
	for _, r := range sdk.OrderNumberers() {
		if !Enabled(ctx, r.PluginSlug, req.ChannelID) {
			continue
		}
		ctx, finish, gate := Instrument(ctx, r.PluginSlug, "order number", req.ChannelID)
		defer finish(&err)
		if gate != nil {
			return "", true, gate
		}
		num, err = r.Numberer(ctx, sdk.OrderNumberRequest{
			ChannelID: req.ChannelID, ChannelSlug: req.ChannelSlug, PlacedAt: req.At,
			Next: func(scope, period string) (int64, error) { return req.Next("plugin:"+r.PluginSlug+":"+scope, period) },
		})
		return num, true, err
	}
	return "", false, nil
}
