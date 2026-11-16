package sdk

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
)

const (
	OrderStatusPending            = "pending"
	OrderStatusConfirmed          = "confirmed"
	OrderStatusOnHold             = "on_hold"
	OrderStatusProcessing         = "processing"
	OrderStatusPartiallyFulfilled = "partially_fulfilled"
	OrderStatusFulfilled          = "fulfilled"
	OrderStatusCancelled          = "cancelled"
)

// A step of a plugin's own inside core's statuses - "awaiting stock" inside confirmed, say. The status still drives
// payment and stock; the stage says where the work stands, and clears when the order moves outside Within
type OrderStage struct {
	Key    string // lowercase, stable - the order keeps pl.Key of it ("<plugin>:awaiting_stock")
	Label  string
	Within []string
}

type RegisteredOrderStage struct {
	PluginSlug string
	Stage      OrderStage
}

// Key is the stage's key as the order keeps it, "<plugin>:<key>"
func (r RegisteredOrderStage) Key() string { return r.PluginSlug + ":" + r.Stage.Key }

var (
	orderStages    registry[RegisteredOrderStage]
	stagedStatuses = []string{OrderStatusPending, OrderStatusConfirmed, OrderStatusOnHold, OrderStatusProcessing,
		OrderStatusPartiallyFulfilled, OrderStatusFulfilled, OrderStatusCancelled}
)

// StagedStatuses is every status a stage can sit in - all but a draft
func StagedStatuses() []string { return slices.Clone(stagedStatuses) }

// Offered on every channel the plugin is enabled for - panics on a malformed stage or a key the plugin already registered
func (pl *Plugin) RegisterOrderStage(s OrderStage) {
	switch {
	case !hookKey.MatchString(s.Key):
		panic("sdk.RegisterOrderStage: key " + s.Key + hookKeyRule)
	case s.Label == "":
		panic("sdk.RegisterOrderStage: " + s.Key + " needs a label")
	case len(s.Within) == 0:
		panic("sdk.RegisterOrderStage: " + s.Key + " needs the statuses it sits in")
	}
	for _, st := range s.Within {
		if !slices.Contains(stagedStatuses, st) {
			panic("sdk.RegisterOrderStage: " + s.Key + " can't sit in " + st + " - a draft has no stage")
		}
	}
	s.Within = slices.Clone(s.Within)
	orderStages.add(RegisteredOrderStage{PluginSlug: pl.slug, Stage: s},
		func(r RegisteredOrderStage) bool { return r.PluginSlug == pl.slug && r.Stage.Key == s.Key },
		"sdk.RegisterOrderStage: "+pl.slug+" already registered "+s.Key)
}

func OrderStages() []RegisteredOrderStage {
	out := orderStages.all()
	for i := range out {
		out[i].Stage.Within = slices.Clone(out[i].Stage.Within)
	}
	return out
}

var ErrOrderStagesUnavailable = errors.New("sdk: order stages unavailable")

type orderStager interface {
	SetOrderStage(ctx context.Context, pluginSlug string, orderID uuid.UUID, key string) error
}

// Moves an order into one of this plugin's stages by the key it registered, or out of one with "" - a stage another
// plugin holds is left alone, and one the order's status doesn't hold is refused. order.transitioning subscribers can
// refuse it too
func (pl *Plugin) SetOrderStage(ctx context.Context, orderID uuid.UUID, key string) error {
	h, ok := hostNow().(orderStager)
	if !ok {
		return ErrOrderStagesUnavailable
	}
	return h.SetOrderStage(ctx, pl.slug, orderID, key)
}
