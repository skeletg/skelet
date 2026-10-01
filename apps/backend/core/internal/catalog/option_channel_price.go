package catalog

import (
	"strings"

	"github.com/google/uuid"
)

// The FIXED amount a surcharge charges on one channel: its override there, else the base when the channel sells in the currency the base is written in. ok=false means nothing sound can be charged, so the pick has to be refused - a zero base is free in every currency
func ResolveChannelFixed(base int32, prices []OptionChannelPrice, channelID uuid.UUID, channelCurrency, baseCurrency string) (int32, bool) {
	for _, p := range prices {
		if p.ChannelID == channelID {
			return p.PriceValue, true
		}
	}
	if base == 0 || strings.EqualFold(channelCurrency, baseCurrency) {
		return base, true
	}
	return 0, false
}

// A value's adjustment on a channel - PERCENT already follows the channel price, a linked value's PriceValue is a FIXED markup on its source
func (v ProductOptionValue) ChannelPriceValue(channelID uuid.UUID, channelCurrency, baseCurrency string) (int32, bool) {
	if v.PriceType == OptionPricePercent && v.SourceProductID == nil {
		return v.PriceValue, true
	}
	return ResolveChannelFixed(v.PriceValue, v.ChannelPrices, channelID, channelCurrency, baseCurrency)
}

// The option-level adjustment of a text, date or file option on a channel - nil when the option carries none
func (o ProductOption) ChannelPriceValue(channelID uuid.UUID, channelCurrency string) (*int32, bool) {
	if o.PriceValue == nil || (o.PriceType != nil && *o.PriceType == OptionPricePercent) {
		return o.PriceValue, true
	}
	v, ok := ResolveChannelFixed(*o.PriceValue, o.ChannelPrices, channelID, channelCurrency, o.Currency)
	return &v, ok
}
