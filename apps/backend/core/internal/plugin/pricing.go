package plugin

import (
	"context"
	"errors"
	"sync"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Per-customer pricing overrides, consulted by the catalog pricing adapter ahead of list and tier
// prices - registration order is install order, and the first plugin to answer wins

type priceResolverReg struct {
	slug string
	r    sdk.PriceResolver
}

var (
	priceMu        sync.RWMutex
	priceResolvers []priceResolverReg
)

func (h *Host) RegisterPriceResolver(pluginSlug string, r sdk.PriceResolver) error {
	if r == nil {
		return errors.New("plugin: RegisterPriceResolver called with nil resolver")
	}
	priceMu.Lock()
	defer priceMu.Unlock()
	priceResolvers = append(priceResolvers, priceResolverReg{slug: pluginSlug, r: r})
	return nil
}

// Whether any plugin prices per customer - lets the caller skip building a request nothing will read
func HasPriceResolvers() bool {
	priceMu.RLock()
	defer priceMu.RUnlock()
	return len(priceResolvers) > 0
}

// The first plugin price for this customer and product, or ok=false to price at catalog
// Fail-soft by contract: a closed kill switch, a timeout, a panic or a resolver error all read as no plugin price, since the alternative is a pricing plugin taking add-to-cart down
func ResolveUnitPrice(ctx context.Context, req sdk.PriceRequest) (int64, bool) {
	priceMu.RLock()
	regs := make([]priceResolverReg, len(priceResolvers))
	copy(regs, priceResolvers)
	priceMu.RUnlock()

	for _, reg := range regs {
		cents, ok := callPriceResolver(ctx, reg, req)
		if ok {
			return cents, true
		}
	}
	return 0, false
}

// Split out so each resolver gets its own deferred finish, rather than every one of them stacking up
// until the loop returns
func callPriceResolver(ctx context.Context, reg priceResolverReg, req sdk.PriceRequest) (int64, bool) {
	ctx, finish, gate := Instrument(ctx, reg.slug, "ResolveUnitPrice", req.ChannelID)
	if gate != nil {
		return 0, false
	}
	var (
		cents int64
		ok    bool
		err   error
	)
	defer func() { finish(&err) }()
	defer func() {
		if rec := recover(); rec != nil {
			err = errFromPanic(rec)
			ok = false
		}
	}()
	cents, ok, err = reg.r.ResolveUnitPrice(ctx, req)
	if err != nil {
		return 0, false
	}
	return cents, ok
}

// Between-scenario reset, matching the other registries
func ResetPriceResolversForTest() {
	priceMu.Lock()
	defer priceMu.Unlock()
	priceResolvers = nil
}
