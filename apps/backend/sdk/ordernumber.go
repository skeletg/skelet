package sdk

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Next draws from core's count for a scope and period you name, the way the store's own format counts
type OrderNumberRequest struct {
	ChannelID   uuid.UUID
	ChannelSlug string
	PlacedAt    time.Time
	Next        func(scope, period string) (int64, error)
}

// Letters, digits and - _ . / #, at most 64 - an error or anything else hands the order to the store's own format
type OrderNumberer func(ctx context.Context, req OrderNumberRequest) (string, error)

type RegisteredOrderNumberer struct {
	PluginSlug string
	Numberer   OrderNumberer
}

var (
	orderNumberersMu sync.Mutex
	orderNumberers   []RegisteredOrderNumberer
)

func (pl *Plugin) RegisterOrderNumberer(fn OrderNumberer) {
	if fn == nil {
		panic("sdk.RegisterOrderNumberer: nil numberer")
	}
	orderNumberersMu.Lock()
	defer orderNumberersMu.Unlock()
	if slices.ContainsFunc(orderNumberers, func(r RegisteredOrderNumberer) bool { return r.PluginSlug == pl.slug }) {
		panic("sdk.RegisterOrderNumberer: " + pl.slug + " already names orders")
	}
	orderNumberers = append(orderNumberers, RegisteredOrderNumberer{PluginSlug: pl.slug, Numberer: fn})
}

func OrderNumberers() []RegisteredOrderNumberer {
	orderNumberersMu.Lock()
	defer orderNumberersMu.Unlock()
	return slices.Clone(orderNumberers)
}
