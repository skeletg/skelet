package aiusage

import (
	"context"
	"math"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type fakeSettings map[string]string

func (f fakeSettings) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return f[path], nil
}

func TestTheCostEstimateUsesTheOperatorsPrices(t *testing.T) {
	used := ai.Usage{Input: 1_000_000, Output: 500_000, CacheRead: 2_000_000, CacheWrite: 400_000}
	for name, tc := range map[string]struct {
		set  fakeSettings
		want float64
		none bool
	}{
		"all four":                    {set: fakeSettings{priceInputPath: "4", priceOutputPath: "$20", priceCacheReadPath: "0.20", priceCacheWritePath: " 5 "}, want: 4 + 10 + 0.4 + 2},
		"blank cache priced as input": {set: fakeSettings{priceInputPath: "1", priceOutputPath: "2"}, want: 1 + 1 + 2 + 0.4},
		"no output price":             {set: fakeSettings{priceInputPath: "4"}, none: true},
		"nothing set":                 {set: fakeSettings{}, none: true},
		"junk":                        {set: fakeSettings{priceInputPath: "four", priceOutputPath: "20"}, none: true},
		"negative":                    {set: fakeSettings{priceInputPath: "-1", priceOutputPath: "20"}, none: true},
		"not a number":                {set: fakeSettings{priceInputPath: "NaN", priceOutputPath: "20"}, none: true},
		"junk cache falls back":       {set: fakeSettings{priceInputPath: "1", priceOutputPath: "2", priceCacheReadPath: "free"}, want: 1 + 1 + 2 + 0.4},
		"free cache reads counts":     {set: fakeSettings{priceInputPath: "1", priceOutputPath: "2", priceCacheReadPath: "0"}, want: 1 + 1 + 0.4},
	} {
		t.Run(name, func(t *testing.T) {
			p, ok := New(nil, tc.set).prices(context.Background())
			if ok == tc.none {
				t.Fatalf("priced = %v, want %v", ok, !tc.none)
			}
			if got := p.cost(used); ok && math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("cost = %v, want %v", got, tc.want)
			}
		})
	}
}
