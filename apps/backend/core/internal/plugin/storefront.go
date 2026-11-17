package plugin

import (
	"context"
	"sync/atomic"

	"github.com/google/uuid"
)

// What purges a record's storefront pages - the cache reactions' own invalidator, wired at boot. Unset, a plugin's
// request does nothing, which is all a host without a storefront cache could do
type storefrontInvalidator func(ctx context.Context, kind string, id uuid.UUID) error

var storefrontInvalidate atomic.Pointer[storefrontInvalidator]

func SetStorefrontInvalidator(fn func(ctx context.Context, kind string, id uuid.UUID) error) {
	inv := storefrontInvalidator(fn)
	storefrontInvalidate.Store(&inv)
}

// Satisfies sdk's storefrontInvalidator, type-asserted rather than added to the Host interface
func (h *Host) InvalidateStorefront(ctx context.Context, kind string, id uuid.UUID) error {
	inv := storefrontInvalidate.Load()
	if inv == nil || *inv == nil {
		return nil
	}
	return (*inv)(ctx, kind, id)
}
