package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *bundleService) GetBundleOption(ctx context.Context, id uuid.UUID) (BundleOption, error) { // the RAW row - no resolved content, no selections, so callers needing the full view go through GetBundleOptions
	return s.repo.GetBundleOptionByID(ctx, id)
}

func (s *bundleService) GetBundleSelection(ctx context.Context, id uuid.UUID) (BundleSelection, error) { // the RAW row, unhydrated
	return s.repo.GetBundleSelectionByID(ctx, id)
}

// ==================== Bundle options ====================

func (s *bundleService) AddBundleOption(ctx context.Context, input AddBundleOptionInput) (BundleOption, error) { // writes the first content row too, so the supplied locale must equal the configured default - Skelet's "initial content uses default locale" rule
	if !input.InputType.IsKnown() {
		return BundleOption{}, fmt.Errorf("%w: %q", ErrInvalidBundleInputType, string(input.InputType))
	}
	if strings.TrimSpace(input.Title) == "" {
		return BundleOption{}, ErrTitleRequired
	}
	parent, err := s.repo.GetProductByID(ctx, input.ProductID)
	if err != nil {
		return BundleOption{}, err
	}
	if parent.Kind != KindBundle {
		return BundleOption{}, fmt.Errorf("%w: product %q is kind %q", ErrBundleParentNotBundle, parent.SKU, string(parent.Kind))
	}
	if strings.TrimSpace(input.Locale) == "" {
		return BundleOption{}, ErrLocaleRequired
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && input.Locale != def {
		return BundleOption{}, fmt.Errorf("%w: got %q, want %q", ErrLocaleMustBeDefault, input.Locale, def)
	}
	if err := validateBundleLimits(input.InputType, input.MinSelections, input.MaxSelections); err != nil {
		return BundleOption{}, err
	}
	return s.repo.CreateBundleOption(ctx, input)
}

func validateBundleLimits(inputType BundleInputType, min int32, max *int32) error { // bounds must be coherent AND compatible with the input type
	if min < 0 {
		return fmt.Errorf("%w: min_selections must be >= 0", ErrInvalidBundleLimits)
	}
	if max != nil {
		if *max < 1 {
			return fmt.Errorf("%w: max_selections must be >= 1", ErrInvalidBundleLimits)
		}
		if *max < min {
			return fmt.Errorf("%w: max_selections (%d) is below min_selections (%d)", ErrInvalidBundleLimits, *max, min)
		}
	}
	if inputType.IsSingleSelect() { // holds at most one selection, so any bound above 1 is incoherent
		if min > 1 {
			return fmt.Errorf("%w: single-select option cannot require %d selections", ErrInvalidBundleLimits, min)
		}
		if max != nil && *max > 1 {
			return fmt.Errorf("%w: single-select option cannot allow %d selections", ErrInvalidBundleLimits, *max)
		}
	}
	return nil
}

func (s *bundleService) UpdateBundleOption(ctx context.Context, input UpdateBundleOptionInput) (BundleOption, error) { // pointer-style "no change = nil" - toggling to single-select with >1 default selection is rejected, since it would leave an ambiguous "which default?" state
	current, err := s.repo.GetBundleOptionByID(ctx, input.ID)
	if err != nil {
		return BundleOption{}, err
	}
	finalInputType := current.InputType
	if input.InputType != nil {
		if !input.InputType.IsKnown() {
			return BundleOption{}, fmt.Errorf("%w: %q", ErrInvalidBundleInputType, string(*input.InputType))
		}
		finalInputType = *input.InputType
	}
	finalRequired := current.IsRequired
	if input.IsRequired != nil {
		finalRequired = *input.IsRequired
	}
	finalSortOrder := current.SortOrder
	if input.SortOrder != nil {
		finalSortOrder = *input.SortOrder
	}
	finalMin := current.MinSelections
	if input.MinSelections != nil {
		finalMin = *input.MinSelections
	}
	finalMax := current.MaxSelections
	if input.ClearMaxSelections {
		finalMax = nil
	} else if input.MaxSelections != nil {
		finalMax = input.MaxSelections
	}
	if err := validateBundleLimits(finalInputType, finalMin, finalMax); err != nil {
		return BundleOption{}, err
	}

	var updated BundleOption
	err = s.inTx(ctx, func(r Repository) error {
		if _, err := r.GetBundleOptionByIDForUpdate(ctx, current.ID); err != nil {
			return err
		}
		n, err := r.CountBundleSelectionDefaultsForOption(ctx, current.ID)
		if err != nil {
			return err
		}
		if err := bundleDefaultsFit(finalInputType, finalMax, n); err != nil {
			return err
		}
		updated, err = r.UpdateBundleOption(ctx, current.ID, finalInputType, finalRequired, finalSortOrder, finalMin, finalMax)
		return err
	})
	return updated, err
}

// A default is pre-picked, so an option can't start with more of them than it lets a shopper pick
func bundleDefaultsFit(t BundleInputType, max *int32, defaults int64) error {
	if t.IsSingleSelect() && defaults > 1 {
		return fmt.Errorf("%w: option has %d default selections", ErrBundleMultiDefaultOnSingle, defaults)
	}
	if max != nil && defaults > int64(*max) {
		return fmt.Errorf("%w: %d defaults, max_selections %d", ErrBundleDefaultsExceedMax, defaults, *max)
	}
	return nil
}

// One more default on optionID must still fit - the option row lock serializes concurrent default flips
func addingBundleDefault(ctx context.Context, r Repository, optionID uuid.UUID) error {
	option, err := r.GetBundleOptionByIDForUpdate(ctx, optionID)
	if err != nil {
		return err
	}
	n, err := r.CountBundleSelectionDefaultsForOption(ctx, optionID)
	if err != nil {
		return err
	}
	return bundleDefaultsFit(option.InputType, option.MaxSelections, n+1)
}

func (s *bundleService) DeleteBundleOption(ctx context.Context, id uuid.UUID) error { // content and selections go with it via FK CASCADE
	if _, err := s.repo.GetBundleOptionByID(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteBundleOption(ctx, id)
}

func (s *bundleService) UpsertBundleOptionContent(ctx context.Context, input UpsertBundleOptionContentInput) (BundleOptionContent, error) { // keyed on (option, locale)
	if strings.TrimSpace(input.Locale) == "" {
		return BundleOptionContent{}, ErrLocaleRequired
	}
	if strings.TrimSpace(input.Title) == "" {
		return BundleOptionContent{}, ErrTitleRequired
	}
	if err := s.ensureLocale(ctx, input.Locale); err != nil {
		return BundleOptionContent{}, err
	}
	if _, err := s.repo.GetBundleOptionByID(ctx, input.OptionID); err != nil {
		return BundleOptionContent{}, err
	}
	return s.repo.UpsertBundleOptionContent(ctx, input)
}

func (s *bundleService) DeleteBundleOptionContent(ctx context.Context, optionID uuid.UUID, locale string) error { // dropping the default-locale row while translations remain is rejected - same rule as ProductContent and CategoryContent
	if err := s.refuseDefaultLocaleDelete(ctx, locale, func() ([]string, error) {
		rows, err := s.repo.ListBundleOptionContent(ctx, optionID)
		return localesOf(rows, bundleContentLocale), err
	}); err != nil {
		return err
	}
	return s.repo.DeleteBundleOptionContent(ctx, optionID, locale)
}

// ==================== Bundle selections ====================

func (s *bundleService) AddBundleSelection(ctx context.Context, input AddBundleSelectionInput) (BundleSelection, error) { // the parent must be a BUNDLE and the linked product SIMPLE (no nested bundles in v1) and not the bundle itself
	if err := validPositiveQuantity(input.Quantity); err != nil {
		return BundleSelection{}, err
	}
	option, err := s.repo.GetBundleOptionByID(ctx, input.OptionID)
	if err != nil {
		return BundleSelection{}, err
	}
	parent, err := s.repo.GetProductByID(ctx, option.ProductID)
	if err != nil {
		return BundleSelection{}, err
	}
	if parent.Kind != KindBundle {
		return BundleSelection{}, fmt.Errorf("%w: product %q is kind %q", ErrBundleParentNotBundle, parent.SKU, string(parent.Kind)) // defensive - options can only be created on BUNDLE products, but the repository doesn't enforce it
	}
	if input.ProductID == parent.ID {
		return BundleSelection{}, fmt.Errorf("%w: %s", ErrBundleSelectionIsParent, parent.SKU)
	}
	child, err := s.repo.GetProductByID(ctx, input.ProductID)
	if err != nil {
		return BundleSelection{}, err
	}
	if child.Kind != KindSimple {
		return BundleSelection{}, fmt.Errorf("%w: product %q is kind %q", ErrBundleSelectionNotSimple, child.SKU, string(child.Kind))
	}

	var created BundleSelection
	err = s.inTx(ctx, func(r Repository) error {
		if input.IsDefault {
			if err := addingBundleDefault(ctx, r, option.ID); err != nil {
				return err
			}
		}
		created, err = r.CreateBundleSelection(ctx, input)
		return err
	})
	return created, err
}

func (s *bundleService) UpdateBundleSelection(ctx context.Context, input UpdateBundleSelectionInput) (BundleSelection, error) { // pointer fields, "no change" = nil - same single-select default-cardinality guard as AddBundleSelection
	current, err := s.repo.GetBundleSelectionByID(ctx, input.ID)
	if err != nil {
		return BundleSelection{}, err
	}
	finalQuantity := current.Quantity
	if input.Quantity != nil {
		if err := validPositiveQuantity(*input.Quantity); err != nil {
			return BundleSelection{}, err
		}
		finalQuantity = *input.Quantity
	}
	finalIsDefault := current.IsDefault
	if input.IsDefault != nil {
		finalIsDefault = *input.IsDefault
	}
	finalSortOrder := current.SortOrder
	if input.SortOrder != nil {
		finalSortOrder = *input.SortOrder
	}

	var updated BundleSelection
	err = s.inTx(ctx, func(r Repository) error {
		if finalIsDefault && !current.IsDefault {
			if err := addingBundleDefault(ctx, r, current.OptionID); err != nil {
				return err
			}
		}
		updated, err = r.UpdateBundleSelection(ctx, current.ID, finalQuantity, finalIsDefault, finalSortOrder)
		return err
	})
	return updated, err
}

func (s *bundleService) DeleteBundleSelection(ctx context.Context, id uuid.UUID) error { // no "required option must keep ≥1 selection" check - that invariant is enforced at add-to-cart time, so admins can hold a half-built bundle in draft
	if _, err := s.repo.GetBundleSelectionByID(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteBundleSelection(ctx, id)
}

// ==================== Read-side: full bundle view ====================

func (s *bundleService) ListBundleSelectionLinksByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]BundleSelectionLink, error) { // batch-resolves the bundle/option/quantity links each product appears in - powers Product.partOf.bundles
	return s.repo.ListBundleSelectionLinksByProductIDs(ctx, productIDs)
}

func (s *bundleService) GetBundleOptions(ctx context.Context, productID uuid.UUID, locale string) ([]BundleOption, error) { // hydrates titles (requested locale, default-locale fallback), selections with their products' display fields, and the full AllContent set per option for admin use
	options, err := s.repo.ListBundleOptionsByProductID(ctx, productID)
	if err != nil {
		return nil, err
	}
	if len(options) == 0 {
		return []BundleOption{}, nil
	}

	defLocale := s.resolveDefaultLocale(ctx)
	if strings.TrimSpace(locale) == "" {
		locale = defLocale
	}

	optionIDs := make([]uuid.UUID, len(options))
	for i, o := range options {
		optionIDs[i] = o.ID
	}

	contentByOption, err := s.repo.ListBundleOptionContentByOptionIDs(ctx, optionIDs)
	if err != nil {
		return nil, err
	}
	selectionsByOption, err := s.repo.ListBundleSelectionsByOptionIDs(ctx, optionIDs)
	if err != nil {
		return nil, err
	}

	var sels []BundleSelection
	for _, os := range selectionsByOption {
		sels = append(sels, os...)
	}
	ids := uniqueIDs(len(sels), func(i int) uuid.UUID { return sels[i].ProductID })
	productByID, contentByProduct, err := s.loadProductsWithContent(ctx, ids, locale, true)
	if err != nil {
		return nil, fmt.Errorf("bundle selections: %w", err)
	}

	for i := range options {
		opt := &options[i]

		opt.AllContent = contentByOption[opt.ID]
		c, _ := pickLocalized(opt.AllContent, bundleContentLocale, locale, defLocale)
		opt.Title = c.Title

		sels := selectionsByOption[opt.ID]
		for j := range sels {
			sel := &sels[j]
			if p, ok := productByID[sel.ProductID]; ok {
				sel.ProductURL = p.URL
				sel.ProductSKU = p.SKU
				sel.ProductDefaultPrice = p.DefaultPrice
				sel.ProductCurrency = p.DefaultCurrencyCode
				if c, ok := contentByProduct[p.ID]; ok {
					sel.ProductName = c.Name
				}
			}
		}
		opt.Selections = sels
	}

	return options, nil
}

func bundleContentLocale(c BundleOptionContent) string { return c.Locale }
