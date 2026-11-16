package plugin

import (
	"context"
	"fmt"

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
			Next: func(ctx context.Context, scope, period string) (int64, error) {
				return req.Next(ctx, "plugin:"+r.PluginSlug+":"+scope, period)
			},
		})
		return num, true, err
	}
	return "", false, nil
}

// ShadowedHooks names the plugins whose numberer or document renderer never runs where an earlier one is on too -
// boot logs it, since nothing else would say why a plugin's numbers or documents never appear
func ShadowedHooks() []string {
	var out []string
	if n := sdk.OrderNumberers(); len(n) > 1 {
		for _, later := range n[1:] {
			out = append(out, fmt.Sprintf("%s names orders only on channels where %s is off", later.PluginSlug, n[0].PluginSlug))
		}
	}
	first := map[sdk.DocumentKind]string{}
	for _, r := range sdk.DocumentRenderers() {
		if earlier, taken := first[r.Kind]; taken {
			out = append(out, fmt.Sprintf("%s makes %s documents only on channels where %s is off", r.PluginSlug, r.Kind, earlier))
			continue
		}
		first[r.Kind] = r.PluginSlug
	}
	return out
}
