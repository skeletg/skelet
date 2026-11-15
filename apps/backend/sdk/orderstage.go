package sdk

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"sync"

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
	Key    string // lowercase, stable - the order keeps it ("awaiting_stock")
	Label  string
	Within []string
}

type RegisteredOrderStage struct {
	PluginSlug string
	Stage      OrderStage
}

var (
	orderStagesMu  sync.Mutex
	orderStages    []RegisteredOrderStage
	orderStageKey  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	stagedStatuses = []string{OrderStatusPending, OrderStatusConfirmed, OrderStatusOnHold, OrderStatusProcessing,
		OrderStatusPartiallyFulfilled, OrderStatusFulfilled, OrderStatusCancelled}
)

// Offered on every channel the plugin is enabled for - panics on a malformed stage or a key another plugin registered
func (pl *Plugin) RegisterOrderStage(s OrderStage) {
	switch {
	case !orderStageKey.MatchString(s.Key):
		panic("sdk.RegisterOrderStage: key " + s.Key + " must be lowercase letters, digits and underscores, starting with a letter")
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
	orderStagesMu.Lock()
	defer orderStagesMu.Unlock()
	if slices.ContainsFunc(orderStages, func(r RegisteredOrderStage) bool { return r.Stage.Key == s.Key }) {
		panic("sdk.RegisterOrderStage: another plugin already registered " + s.Key)
	}
	s.Within = slices.Clone(s.Within)
	orderStages = append(orderStages, RegisteredOrderStage{PluginSlug: pl.slug, Stage: s})
}

func OrderStages() []RegisteredOrderStage {
	orderStagesMu.Lock()
	defer orderStagesMu.Unlock()
	out := slices.Clone(orderStages)
	for i := range out {
		out[i].Stage.Within = slices.Clone(out[i].Stage.Within)
	}
	return out
}

var ErrOrderStagesUnavailable = errors.New("sdk: order stages unavailable")

type orderStager interface {
	SetOrderStage(ctx context.Context, pluginSlug string, orderID uuid.UUID, key string) error
}

// Moves an order into one of this plugin's stages, or out of one with "" - another plugin's stage is refused, and so is
// one the order's status doesn't hold. order.transitioning subscribers can refuse it too
func (pl *Plugin) SetOrderStage(ctx context.Context, orderID uuid.UUID, key string) error {
	h, ok := hostNow().(orderStager)
	if !ok {
		return ErrOrderStagesUnavailable
	}
	return h.SetOrderStage(ctx, pl.slug, orderID, key)
}
