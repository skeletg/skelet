package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
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

// A configurable selection says so, for the picker and the health check - every other row is the SIMPLE it has to be
func selectionKind(s catalog.BundleSelection) model.ProductKind {
	if s.ProductKind == "" {
		return model.ProductKindSimple
	}
	return model.ProductKind(strings.ToUpper(string(s.ProductKind)))
}

func toBundleSelectionModel(s catalog.BundleSelection) *model.BundleSelection {
	prod := &model.Product{
		ID:                  s.ProductID.String(),
		URL:                 s.ProductURL,
		Sku:                 s.ProductSKU,
		Kind:                selectionKind(s),
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

var errEmptyBundleOp = apperr.New("BUNDLE_EDIT_EMPTY", apperr.ClassValidation, "a bundle edit has to name one change")

// The bundle's own options and selections by id, then tempIds as adds create them - an id from any other bundle reads as
// not found, so one call can't reach past the product it was gated on
type bundleRefs struct {
	options, selections map[string]uuid.UUID
}

func (b bundleRefs) option(ref string) (uuid.UUID, error) {
	if id, ok := b.options[ref]; ok {
		return id, nil
	}
	return uuid.Nil, fmt.Errorf("%w: %s", catalog.ErrBundleOptionNotFound, ref)
}

func (b bundleRefs) selection(ref string) (uuid.UUID, error) {
	if id, ok := b.selections[ref]; ok {
		return id, nil
	}
	return uuid.Nil, fmt.Errorf("%w: %s", catalog.ErrBundleSelectionNotFound, ref)
}

func (r *mutationResolver) applyBundleOps(ctx context.Context, productID uuid.UUID, ops []*model.BundleStructureOpInput) error {
	current, err := r.CatalogSvc.GetBundleOptions(ctx, productID, "")
	if err != nil {
		return err
	}
	refs := bundleRefs{options: map[string]uuid.UUID{}, selections: map[string]uuid.UUID{}}
	for _, o := range current {
		refs.options[o.ID.String()] = o.ID
		for _, s := range o.Selections {
			refs.selections[s.ID.String()] = s.ID
		}
	}
	return r.CatalogSvc.EditBundle(ctx, func(e catalog.BundleEditor) error {
		for i, op := range ops {
			if err := applyBundleOp(ctx, e, productID, refs, op); err != nil {
				return fmt.Errorf("bundle edit %d: %w", i+1, err)
			}
		}
		return nil
	})
}

func applyBundleOp(ctx context.Context, e catalog.BundleEditor, productID uuid.UUID, refs bundleRefs, op *model.BundleStructureOpInput) error {
	name := func(id uuid.UUID, into map[string]uuid.UUID) {
		if op.TempID != nil {
			into[*op.TempID] = id
		}
		into[id.String()] = id
	}
	switch {
	case op.AddOption != nil:
		in := op.AddOption
		if in.ProductID != productID.String() {
			return fmt.Errorf("%w: option for another product", catalog.ErrBundleOptionNotFound)
		}
		opt, err := e.AddBundleOption(ctx, catalog.AddBundleOptionInput{
			ProductID:     productID,
			InputType:     catalog.BundleInputType(strings.ToLower(string(in.InputType))),
			IsRequired:    boolOrDefault(in.IsRequired, true),
			SortOrder:     int32Or(in.SortOrder, 0),
			MinSelections: int32Or(in.MinSelections, 0),
			MaxSelections: in.MaxSelections,
			Locale:        in.Locale,
			Title:         in.Title,
		})
		if err == nil {
			name(opt.ID, refs.options)
		}
		return err
	case op.UpdateOption != nil:
		in := op.UpdateOption
		id, err := refs.option(in.ID)
		if err != nil {
			return err
		}
		upd := catalog.UpdateBundleOptionInput{
			ID: id, IsRequired: in.IsRequired, SortOrder: in.SortOrder, MinSelections: in.MinSelections, MaxSelections: in.MaxSelections,
			ClearMaxSelections: in.ClearMaxSelections != nil && *in.ClearMaxSelections,
		}
		if in.InputType != nil {
			t := catalog.BundleInputType(strings.ToLower(string(*in.InputType)))
			upd.InputType = &t
		}
		_, err = e.UpdateBundleOption(ctx, upd)
		return err
	case op.DeleteOption != nil:
		id, err := refs.option(*op.DeleteOption)
		if err != nil {
			return err
		}
		return e.DeleteBundleOption(ctx, id)
	case op.UpsertContent != nil:
		id, err := refs.option(op.UpsertContent.OptionID)
		if err != nil {
			return err
		}
		title := ""
		if op.UpsertContent.Title != nil {
			title = *op.UpsertContent.Title
		}
		_, err = e.UpsertBundleOptionContent(ctx, catalog.UpsertBundleOptionContentInput{OptionID: id, Locale: op.UpsertContent.Locale, Title: title})
		return err
	case op.DeleteContent != nil:
		id, err := refs.option(op.DeleteContent.OptionID)
		if err != nil {
			return err
		}
		return e.DeleteBundleOptionContent(ctx, id, op.DeleteContent.Locale)
	case op.AddSelection != nil:
		in := op.AddSelection
		optionID, err := refs.option(in.OptionID)
		if err != nil {
			return err
		}
		pid, err := parseID(in.ProductID, "product")
		if err != nil {
			return err
		}
		sel, err := e.AddBundleSelection(ctx, catalog.AddBundleSelectionInput{
			OptionID: optionID, ProductID: pid,
			Quantity:  float64Or(in.Quantity, catalog.DefaultStructureQuantity),
			IsDefault: boolOrFalse(in.IsDefault),
			SortOrder: int32Or(in.SortOrder, 0),
		})
		if err == nil {
			name(sel.ID, refs.selections)
		}
		return err
	case op.UpdateSelection != nil:
		in := op.UpdateSelection
		id, err := refs.selection(in.ID)
		if err != nil {
			return err
		}
		_, err = e.UpdateBundleSelection(ctx, catalog.UpdateBundleSelectionInput{ID: id, Quantity: in.Quantity, IsDefault: in.IsDefault, SortOrder: in.SortOrder})
		return err
	case op.DeleteSelection != nil:
		id, err := refs.selection(*op.DeleteSelection)
		if err != nil {
			return err
		}
		return e.DeleteBundleSelection(ctx, id)
	}
	return errEmptyBundleOp
}
