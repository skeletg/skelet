package sdk

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// A kind of record the storefront has pages for
type StorefrontKind string

const (
	StorefrontProduct    StorefrontKind = "product"
	StorefrontCategory   StorefrontKind = "category"
	StorefrontCollection StorefrontKind = "collection"
	StorefrontPage       StorefrontKind = "cms-page"
)

var ErrStorefrontUnavailable = errors.New("sdk: storefront invalidation unavailable")

type storefrontInvalidator interface {
	InvalidateStorefront(ctx context.Context, kind string, id uuid.UUID) error
}

// InvalidateStorefront purges a record's storefront pages and cached answers, the way core's own change to the record
// does - call it after your data on the record changed, since core sends no event about a plugin's table. It follows
// the store's cache settings, so with product caching off a product's purge does nothing
func InvalidateStorefront(ctx context.Context, kind StorefrontKind, id uuid.UUID) error {
	h, ok := hostNow().(storefrontInvalidator)
	if !ok {
		return ErrStorefrontUnavailable
	}
	return h.InvalidateStorefront(ctx, string(kind), id)
}
