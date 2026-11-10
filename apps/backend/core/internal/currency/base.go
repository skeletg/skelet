package currency

import (
	"context"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type settingReader interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

// BaseCode is the currency every stored rate is quoted against - the default when unset, an error when the setting
// won't read, so nothing quietly converts against the wrong base
func BaseCode(ctx context.Context, cfg settingReader) (string, error) {
	base, err := cfg.GetString(ctx, config.BaseCurrencyPath, config.DefaultScope)
	if err != nil {
		return "", fmt.Errorf("read base currency: %w", err)
	}
	if base == "" {
		return config.DefaultBaseCurrency, nil
	}
	return base, nil
}

type codeLookup interface {
	GetByCode(ctx context.Context, code string) (Currency, error)
}

// ToBase converts minor units into the base currency - false when a rate is missing
type ToBase func(cents int64) (int64, bool)

// BaseRates converts amounts into the base currency at the stored rates, for figures stamped as they happen since no
// rate history is kept to convert by later
type BaseRates struct {
	cfg settingReader
	cur codeLookup
}

func NewBaseRates(cfg settingReader, cur codeLookup) *BaseRates {
	return &BaseRates{cfg: cfg, cur: cur}
}

// From reads the base and both rates once, for every amount of one order or event - a conversion that always reports
// false when any of them is missing
func (b *BaseRates) From(ctx context.Context, code string) ToBase {
	unknown := func(int64) (int64, bool) { return 0, false }
	base, err := BaseCode(ctx, b.cfg)
	if err != nil {
		return unknown
	}
	if normalizeCode(base) == normalizeCode(code) {
		return func(c int64) (int64, bool) { return c, true }
	}
	from, err := b.cur.GetByCode(ctx, code)
	if err != nil {
		return unknown
	}
	to, err := b.cur.GetByCode(ctx, base)
	if err != nil {
		return unknown
	}
	return func(c int64) (int64, bool) { return Convert(c, from, to) }
}
