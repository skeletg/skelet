package sdk

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Next draws from core's count for a scope and period you name, the way the store's own format counts
type OrderNumberRequest struct {
	ChannelID   uuid.UUID
	ChannelSlug string
	PlacedAt    time.Time
	Next        func(ctx context.Context, scope, period string) (int64, error)
}

// Letters, digits and - _ . / #, at most 64 - an error or anything else hands the order to the store's own format
type OrderNumberer func(ctx context.Context, req OrderNumberRequest) (string, error)

type RegisteredOrderNumberer struct {
	PluginSlug string
	Numberer   OrderNumberer
}

var orderNumberers registry[RegisteredOrderNumberer]

func (pl *Plugin) RegisterOrderNumberer(fn OrderNumberer) {
	if fn == nil {
		panic("sdk.RegisterOrderNumberer: nil numberer")
	}
	orderNumberers.add(RegisteredOrderNumberer{PluginSlug: pl.slug, Numberer: fn},
		func(r RegisteredOrderNumberer) bool { return r.PluginSlug == pl.slug }, "sdk.RegisterOrderNumberer: "+pl.slug+" already names orders")
}

func OrderNumberers() []RegisteredOrderNumberer { return orderNumberers.all() }
