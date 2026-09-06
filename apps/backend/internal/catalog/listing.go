package catalog

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/pricing"
)

type ProductChannelListing struct { // per-(product, channel) commerce row - a listing existing means the product is sold on that channel, and Price=nil inherits products.default_price but only when the channel currency matches products.default_currency_code (enforced at the service layer)
	ProductID      uuid.UUID
	ChannelID      uuid.UUID
	Price          *int64 // nil = inherit
	IsPublished    bool
	URL            *string // per-channel override, nil = inherit the product's global url - unique within the channel (partial index)
	CompareAtPrice *int64  // per-channel override, independent of the special trio below - nil = inherit
	SpecialPrice   *int64  // setting this replaces the product-level special trio wholesale - nil = inherit
	SpecialFrom    *time.Time
	SpecialTo      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ProductChannelListingInput struct {
	ProductID      uuid.UUID
	ChannelID      uuid.UUID
	Price          *int64
	IsPublished    bool
	URL            *string
	CompareAtPrice *int64
	SpecialPrice   *int64
	SpecialFrom    *time.Time
	SpecialTo      *time.Time
}

var (
	ErrListingNotFound         = errors.New("product store listing not found")
	ErrListingCurrencyMismatch = errors.New("listing cannot inherit price: store currency differs from product default currency")
)

// Listing price overrides the base, a listing that sets SpecialPrice replaces the product-level special trio wholesale, and a listing CompareAtPrice overrides independently - every consumer of "what does this cost on this channel" (cart pricing, front render, merchant feed, admin effectivePrice) folds through here
func EffectivePricingInputs(p Product, l *ProductChannelListing) pricing.Inputs {
	in := pricing.Inputs{
		BasePrice:   p.DefaultPrice,
		CompareAt:   p.CompareAtPrice,
		Special:     p.SpecialPrice,
		SpecialFrom: p.SpecialFrom,
		SpecialTo:   p.SpecialTo,
	}
	if l != nil {
		if l.Price != nil {
			in.BasePrice = *l.Price
		}
		if l.CompareAtPrice != nil {
			in.CompareAt = l.CompareAtPrice
		}
		if l.SpecialPrice != nil {
			in.Special = l.SpecialPrice
			in.SpecialFrom = l.SpecialFrom
			in.SpecialTo = l.SpecialTo
		}
	}
	return in
}
