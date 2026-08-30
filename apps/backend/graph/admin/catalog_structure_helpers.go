package admin

import (
	"strings"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/catalog"
)

// Selections render as slim Product stand-ins carrying just what the dashboard editor needs - they're SIMPLE products by invariant, so there's no nested bundle data to load
func toBundleOptionModels(opts []catalog.BundleOption) []*model.BundleOption {
	out := make([]*model.BundleOption, len(opts))
	for i, o := range opts {
		out[i] = toBundleOptionModel(o)
	}
	return out
}

func toBundleOptionModel(o catalog.BundleOption) *model.BundleOption {
	content := make([]*model.BundleOptionContent, len(o.AllContent))
	for i, c := range o.AllContent {
		content[i] = &model.BundleOptionContent{Locale: c.Locale, Title: c.Title}
	}
	selections := make([]*model.BundleSelection, len(o.Selections))
	for i, s := range o.Selections {
		selections[i] = toBundleSelectionModel(s)
	}
	return &model.BundleOption{
		ID:            o.ID.String(),
		InputType:     model.BundleInputType(strings.ToUpper(string(o.InputType))),
		IsRequired:    o.IsRequired,
		SortOrder:     o.SortOrder,
		MinSelections: o.MinSelections,
		MaxSelections: o.MaxSelections,
		Title:         o.Title,
		Content:       content,
		Selections:    selections,
		CreatedAt:     o.CreatedAt,
		UpdatedAt:     o.UpdatedAt,
	}
}

func toBundleSelectionModel(s catalog.BundleSelection) *model.BundleSelection {
	prod := &model.Product{
		ID:                  s.ProductID.String(),
		URL:                 s.ProductURL,
		Sku:                 s.ProductSKU,
		Kind:                model.ProductKindSimple,
		DefaultPrice:        int32(s.ProductDefaultPrice),
		DefaultCurrencyCode: s.ProductCurrency,
	}
	if s.ProductName != "" {
		name := s.ProductName
		prod.Name = &name
	}
	return &model.BundleSelection{
		ID:        s.ID.String(),
		Product:   prod,
		Quantity:  s.Quantity,
		IsDefault: s.IsDefault,
		SortOrder: s.SortOrder,
	}
}

// Same slim stand-in as bundle selections - components are SIMPLE products by invariant
func toKitComponentModels(comps []catalog.KitComponent) []*model.KitComponent {
	out := make([]*model.KitComponent, len(comps))
	for i, c := range comps {
		out[i] = toKitComponentModel(c)
	}
	return out
}

func toKitComponentModel(c catalog.KitComponent) *model.KitComponent {
	prod := &model.Product{
		ID:                  c.ComponentProductID.String(),
		URL:                 c.ComponentURL,
		Sku:                 c.ComponentSKU,
		Kind:                model.ProductKindSimple,
		DefaultPrice:        int32(c.ComponentDefaultPrice),
		DefaultCurrencyCode: c.ComponentCurrency,
	}
	if c.ComponentName != "" {
		name := c.ComponentName
		prod.Name = &name
	}
	return &model.KitComponent{
		ID:        c.ID.String(),
		Component: prod,
		Quantity:  c.Quantity,
		SortOrder: c.SortOrder,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}
