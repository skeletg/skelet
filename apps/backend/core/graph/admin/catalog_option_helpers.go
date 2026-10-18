package admin

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Values only exist for the select family - for text/date/file the option's own priceValue/priceType carry the adjustment
func toProductOptionModels(opts []catalog.ProductOption, perm auth.PermittedChannels) []*model.ProductOption {
	out := make([]*model.ProductOption, len(opts))
	for i, o := range opts {
		out[i] = toProductOptionModel(o, perm)
	}
	return out
}

func toOptionChannelPriceModels(prices []catalog.OptionChannelPrice, perm auth.PermittedChannels) []*model.OptionChannelPrice {
	out := make([]*model.OptionChannelPrice, 0, len(prices))
	for _, p := range prices {
		if perm.Allows(p.ChannelID) {
			out = append(out, &model.OptionChannelPrice{ChannelID: p.ChannelID.String(), PriceValue: p.PriceValue})
		}
	}
	return out
}

func toProductOptionModel(o catalog.ProductOption, perm auth.PermittedChannels) *model.ProductOption {
	content := make([]*model.ProductOptionContent, len(o.AllContent))
	for i, c := range o.AllContent {
		content[i] = &model.ProductOptionContent{Locale: c.Locale, Title: c.Title, HelpText: c.HelpText}
	}
	values := make([]*model.ProductOptionValue, len(o.Values))
	for i, v := range o.Values {
		values[i] = toProductOptionValueModel(v, perm)
	}
	m := &model.ProductOption{
		ID:            o.ID.String(),
		InputType:     model.ProductOptionInputType(strings.ToUpper(string(o.InputType))),
		IsRequired:    o.IsRequired,
		SortOrder:     o.SortOrder,
		Title:         o.Title,
		HelpText:      o.HelpText,
		Content:       content,
		Values:        values,
		ChannelPrices: toOptionChannelPriceModels(o.ChannelPrices, perm),
		CreatedAt:     o.CreatedAt,
		UpdatedAt:     o.UpdatedAt,
	}
	if o.PriceValue != nil {
		v := *o.PriceValue
		m.PriceValue = &v
	}
	if o.PriceType != nil {
		t := model.ProductOptionPriceType(strings.ToUpper(string(*o.PriceType)))
		m.PriceType = &t
	}
	if o.MaxChars != nil {
		v := *o.MaxChars
		m.MaxChars = &v
	}
	if o.MaxFileBytes != nil {
		v := int32(*o.MaxFileBytes)
		m.MaxFileBytes = &v
	}
	if o.FileExtensions != "" {
		s := o.FileExtensions
		m.FileExtensions = &s
	}
	if o.VisibleIf != nil {
		m.VisibleIf = visibleIfToAny(o.VisibleIf)
	}
	return m
}

// Round-tripped through JSON so the scalar emits plain maps and arrays with the struct tags' snake_case keys
func visibleIfToAny(g *catalog.OptionConditionGroup) any {
	raw, err := json.Marshal(g)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// Unknown keys are rejected, so a typo'd op or key fails at the API edge instead of silently never matching
func visibleIfFromInput(v any) (*catalog.OptionConditionGroup, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, errInvalidInput("visibleIf", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var g catalog.OptionConditionGroup
	if err := dec.Decode(&g); err != nil {
		return nil, errInvalidInput("visibleIf", err)
	}
	return &g, nil
}

func toProductOptionValueModel(v catalog.ProductOptionValue, perm auth.PermittedChannels) *model.ProductOptionValue {
	content := make([]*model.ProductOptionValueContent, len(v.AllContent))
	for i, c := range v.AllContent {
		content[i] = &model.ProductOptionValueContent{Locale: c.Locale, Title: c.Title}
	}
	m := &model.ProductOptionValue{
		ID:            v.ID.String(),
		SortOrder:     v.SortOrder,
		PriceValue:    v.PriceValue,
		PriceType:     model.ProductOptionPriceType(strings.ToUpper(string(v.PriceType))),
		IsDefault:     v.IsDefault,
		Quantity:      v.Quantity,
		Title:         v.Title,
		Content:       content,
		ChannelPrices: toOptionChannelPriceModels(v.ChannelPrices, perm),
		CreatedAt:     v.CreatedAt,
		UpdatedAt:     v.UpdatedAt,
	}
	if v.ForcedLineQty != nil {
		q := *v.ForcedLineQty
		m.ForcedLineQty = &q
	}
	if v.SourceProductID != nil {
		prod := &model.Product{
			ID:                  v.SourceProductID.String(),
			URL:                 v.SourceProductURL,
			Sku:                 v.SourceProductSKU,
			Kind:                model.ProductKindSimple,
			DefaultPrice:        int32(v.SourceProductDefaultPrice),
			DefaultCurrencyCode: v.SourceProductCurrency,
		}
		if v.SourceProductName != "" {
			name := v.SourceProductName
			prod.Name = &name
		}
		m.SourceProduct = prod
	}
	return m
}
