package front

import (
	"context"
	"sort"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/country"
)

func countryScope(ctx context.Context) config.Scope { // channel scope when a channel is bound to the context, else the default scope
	if id := channelIDFromContext(ctx); id != nil {
		return config.Scope{Kind: config.ScopeChannel, ChannelID: id}
	}
	return config.DefaultScope
}

func sortedSet(set map[string]bool) []string { // deterministic order - GraphQL lists should be stable across requests
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The DefaultCurrency lookup is best-effort - a missing or disabled currency surfaces as a null relation, since the FK is ON DELETE SET NULL
func (r *Resolver) toCountryModel(ctx context.Context, c country.Country) *model.Country {
	m := &model.Country{
		Code:                c.Code,
		Name:                c.Name,
		NativeName:          c.NativeName,
		EmojiFlag:           c.EmojiFlag,
		PhoneCode:           c.PhoneCode,
		DefaultCurrencyCode: c.DefaultCurrencyCode,
		IsActive:            c.IsActive,
		SortOrder:           int32(c.SortOrder),
		Metadata:            toStringMapItems(c.Metadata),
		CreatedAt:           c.CreatedAt,
		UpdatedAt:           c.UpdatedAt,
	}
	if c.DefaultCurrencyCode != "" {
		if cur, err := r.CurrencySvc.GetByCode(ctx, c.DefaultCurrencyCode); err == nil { // a missing or transient lookup leaves DefaultCurrency unset rather than failing the render
			m.DefaultCurrency = toCurrencyModel(cur)
		}
	}
	return m
}
