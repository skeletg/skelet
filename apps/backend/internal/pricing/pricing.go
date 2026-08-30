// Package pricing resolves a product's effective sell price from its base, compare-at and scheduled special prices - pure functions, so cart pricing, front render, admin effectivePrice and the merchant feed all share one definition of "what does it cost right now"
package pricing

import "time"

// The price fields after channel-listing overrides have been folded in by the caller - a listing setting special_price replaces the product-level special trio wholesale, and compare_at follows the same rule
type Inputs struct {
	BasePrice   int64 // listing price override if set, else product default_price
	CompareAt   *int64
	Special     *int64
	SpecialFrom *time.Time
	SpecialTo   *time.Time
}

type Resolved struct {
	Price int64
	// The strike-through price to show when higher than Price - the explicit compare-at when set, else the base price while a special is active - Nil when there's nothing meaningful to strike
	CompareAt *int64
	OnSale    bool
}

// Whether a special price applies at now - a nil bound is open-ended on that side
func SpecialActive(special *int64, from, to *time.Time, now time.Time) bool {
	if special == nil {
		return false
	}
	if from != nil && now.Before(*from) {
		return false
	}
	if to != nil && !now.Before(*to) {
		return false
	}
	return true
}

func Resolve(in Inputs, now time.Time) Resolved {
	out := Resolved{Price: in.BasePrice}
	if SpecialActive(in.Special, in.SpecialFrom, in.SpecialTo, now) && *in.Special < in.BasePrice {
		out.Price = *in.Special
		out.OnSale = true
		base := in.BasePrice
		out.CompareAt = &base
	}
	// An explicit compare-at wins over the implied base strike-through, but only when it actually exceeds the sell price - one at or below the price is noise, not a saving
	if in.CompareAt != nil && *in.CompareAt > out.Price {
		v := *in.CompareAt
		out.CompareAt = &v
	}
	return out
}

// One quantity-break price row, optionally scoped to a customer group and/or channel, where nil applies to all
type Tier struct {
	MinQty          float64
	Price           int64
	CustomerGroupID *string // uuid string; nil = every group
	ChannelID       *string // uuid string; nil = every channel
}

// The lowest applicable tier price for the given quantity, customer group and channel, or nil when none applies - a tier applies when qty >= MinQty and its scopes match, with a nil scope matching
func BestTier(tiers []Tier, qty float64, customerGroupID *string, channelID string) *int64 {
	var best *int64
	for i := range tiers {
		t := tiers[i]
		if qty < t.MinQty {
			continue
		}
		if t.ChannelID != nil && *t.ChannelID != channelID {
			continue
		}
		if t.CustomerGroupID != nil {
			if customerGroupID == nil || *t.CustomerGroupID != *customerGroupID {
				continue
			}
		}
		if best == nil || t.Price < *best {
			p := t.Price
			best = &p
		}
	}
	return best
}
