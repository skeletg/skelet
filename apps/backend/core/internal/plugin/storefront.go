package plugin

import (
	"context"

	"github.com/google/uuid"
)

// What purges a record's storefront pages - the cache reactions' own invalidator, wired at boot. Unset, a plugin's
// request does nothing, which is all a host without a storefront cache could do
var storefrontInvalidate func(ctx context.Context, kind string, id uuid.UUID) error

func SetStorefrontInvalidator(fn func(ctx context.Context, kind string, id uuid.UUID) error) {
	storefrontInvalidate = fn
}

// Satisfies sdk's storefrontInvalidator, type-asserted rather than added to the Host interface
func (h *Host) InvalidateStorefront(ctx context.Context, kind string, id uuid.UUID) error {
	if storefrontInvalidate == nil {
		return nil
	}
	return storefrontInvalidate(ctx, kind, id)
}
