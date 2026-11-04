package activity

import (
	"context"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type configReader interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

type currencyLookup interface {
	GetByCode(ctx context.Context, code string) (currency.Currency, error)
}

// BaseConverter converts an amount into the platform's base currency at the stored rate, for the amounts the spine
// stamps as they happen - false when a rate is missing, and the event goes without its base amount
func BaseConverter(cfg configReader, cur currencyLookup) ToBase {
	return func(ctx context.Context, cents int64, code string) (int64, bool) {
		base, err := cfg.GetString(ctx, config.BaseCurrencyPath, config.DefaultScope)
		if err != nil || base == "" {
			base = config.DefaultBaseCurrency
		}
		if strings.EqualFold(base, code) {
			return cents, true
		}
		from, err := cur.GetByCode(ctx, code)
		if err != nil {
			return 0, false
		}
		to, err := cur.GetByCode(ctx, base)
		if err != nil {
			return 0, false
		}
		return currency.Convert(cents, from, to)
	}
}
