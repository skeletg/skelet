package catalog_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

func TestChannelSurchargeResolution(t *testing.T) {
	usd, eur := uuid.New(), uuid.New()
	src := uuid.New()
	overridden := []catalog.OptionChannelPrice{{ChannelID: eur, PriceValue: 450}}
	fixed := catalog.OptionPriceFixed
	percent := catalog.OptionPricePercent
	pv := func(v int32) *int32 { return &v }

	for name, tc := range map[string]struct {
		got    func() (int32, bool)
		want   int32
		wantOK bool
	}{
		"override wins on its channel": {func() (int32, bool) {
			return catalog.ProductOptionValue{PriceValue: 500, PriceType: fixed, ChannelPrices: overridden}.ChannelPriceValue(eur, "EUR", "USD")
		}, 450, true},
		"base applies in the product's currency": {func() (int32, bool) {
			return catalog.ProductOptionValue{PriceValue: 500, PriceType: fixed, ChannelPrices: overridden}.ChannelPriceValue(usd, "usd", "USD")
		}, 500, true},
		"other currency without override refuses": {func() (int32, bool) {
			return catalog.ProductOptionValue{PriceValue: 500, PriceType: fixed}.ChannelPriceValue(eur, "EUR", "USD")
		}, 0, false},
		"zero is free everywhere": {func() (int32, bool) {
			return catalog.ProductOptionValue{PriceType: fixed}.ChannelPriceValue(eur, "EUR", "USD")
		}, 0, true},
		"percent follows the channel price": {func() (int32, bool) {
			return catalog.ProductOptionValue{PriceValue: 10, PriceType: percent}.ChannelPriceValue(eur, "EUR", "USD")
		}, 10, true},
		"a linked value's markup is fixed": {func() (int32, bool) {
			return catalog.ProductOptionValue{PriceValue: 50, PriceType: fixed, SourceProductID: &src}.ChannelPriceValue(eur, "EUR", "USD")
		}, 0, false},
		"option-level override": {func() (int32, bool) {
			v, ok := catalog.ProductOption{PriceValue: pv(300), PriceType: &fixed, Currency: "USD", ChannelPrices: overridden}.ChannelPriceValue(eur, "EUR")
			return *v, ok
		}, 450, true},
		"option-level other currency refuses": {func() (int32, bool) {
			v, ok := catalog.ProductOption{PriceValue: pv(300), PriceType: &fixed, Currency: "USD"}.ChannelPriceValue(eur, "EUR")
			return *v, ok
		}, 0, false},
		"option-level percent": {func() (int32, bool) {
			v, ok := catalog.ProductOption{PriceValue: pv(15), PriceType: &percent, Currency: "USD"}.ChannelPriceValue(eur, "EUR")
			return *v, ok
		}, 15, true},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := tc.got()
			if ok != tc.wantOK || (ok && got != tc.want) {
				t.Fatalf("got (%d, %v), want (%d, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
	if v, ok := (catalog.ProductOption{Currency: "USD"}).ChannelPriceValue(eur, "EUR"); v != nil || !ok {
		t.Fatalf("an option with no price = (%v, %v), want (nil, true)", v, ok)
	}
}
