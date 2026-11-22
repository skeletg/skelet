package currency

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type baseSetting struct {
	code string
	err  error
}

func (b baseSetting) GetString(context.Context, string, config.Scope) (string, error) {
	return b.code, b.err
}

type codes map[string]float64

func (c codes) GetByCode(_ context.Context, code string) (Currency, error) {
	r, ok := c[code]
	if !ok {
		return Currency{}, sql.ErrNoRows
	}
	return Currency{Code: code, ExchangeRate: &r}, nil
}

func TestBaseRatesConvertAtTheStoredRateOrNotAtAll(t *testing.T) {
	ctx := context.Background()
	rates := codes{"EUR": 1, "RSD": 117}
	cases := []struct {
		name    string
		setting baseSetting
		code    string
		want    int64
		ok      bool
	}{
		{"into the base", baseSetting{code: "EUR"}, "RSD", 1000, true},
		{"already the base, any case", baseSetting{code: "eur"}, "EUR", 117000, true},
		{"unset base is the default", baseSetting{}, config.DefaultBaseCurrency, 117000, true},
		{"no rate for the currency", baseSetting{code: "EUR"}, "USD", 0, false},
		{"a setting that won't read converts nothing", baseSetting{err: errors.New("down")}, "RSD", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NewBaseRates(tc.setting, rates).From(ctx, tc.code)(117000)
			if got != tc.want || ok != tc.ok {
				t.Errorf("= %d, %v, want %d, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
