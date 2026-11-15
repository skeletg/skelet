package plugin

import (
	"context"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func OrderStagesFor(ctx context.Context, channelID uuid.UUID) []order.Stage {
	var out []order.Stage
	for _, r := range sdk.OrderStages() {
		if Enabled(ctx, r.PluginSlug, channelID) {
			out = append(out, orderStage(r))
		}
	}
	return out
}

// AllOrderStages is every compiled-in plugin's stages, enabled anywhere or not - for labelling and filtering across channels
func AllOrderStages() []order.Stage {
	regs := sdk.OrderStages()
	out := make([]order.Stage, len(regs))
	for i, r := range regs {
		out[i] = orderStage(r)
	}
	return out
}

func orderStage(r sdk.RegisteredOrderStage) order.Stage {
	return order.Stage{Key: r.Stage.Key, Label: r.Stage.Label, Within: r.Stage.Within, Plugin: r.PluginSlug}
}

type OrderStager interface {
	SetStage(ctx context.Context, orderID uuid.UUID, key, plugin string) (order.Order, error)
}

var orderStager atomic.Pointer[OrderStager]

func SetOrderStager(s OrderStager) {
	if installedHost.Load() == nil {
		panic("plugin.SetOrderStager called before plugin.Install")
	}
	if s == nil {
		panic("plugin.SetOrderStager called with a nil stager")
	}
	if !orderStager.CompareAndSwap(nil, &s) {
		panic("plugin.SetOrderStager called twice")
	}
}

func (h *Host) SetOrderStage(ctx context.Context, pluginSlug string, orderID uuid.UUID, key string) error {
	s := orderStager.Load()
	if s == nil {
		return sdk.ErrOrderStagesUnavailable
	}
	_, err := (*s).SetStage(ctx, orderID, key, pluginSlug)
	return err
}
