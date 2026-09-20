package catalog

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// GetProductOption returns the raw option row (no resolved content, no values) - Callers that need the full view should go through GetProductOptions instead
func (s *optionService) GetProductOption(ctx context.Context, id uuid.UUID) (ProductOption, error) {
	return s.repo.GetProductOptionByID(ctx, id)
}

// GetProductOptionValue returns the raw value row
func (s *optionService) GetProductOptionValue(ctx context.Context, id uuid.UUID) (ProductOptionValue, error) {
	return s.repo.GetProductOptionValueByID(ctx, id)
}

// ==================== Options ====================

// AddProductOption attaches one custom option (e.g. "Engrave a name", "Pick wrapping colour", "Upload artwork") to a product - Only simple and configurable parents are allowed: the cart resolves options for simples and merges a configurable parent's set into its variants, while every other kind would carry dead config that is never enforced or priced - Initial-content locale must equal the configured default locale (matches the rule used for bundle options + products)
func (s *optionService) AddProductOption(ctx context.Context, input AddProductOptionInput) (ProductOption, error) {
	if !input.InputType.IsKnown() {
		return ProductOption{}, fmt.Errorf("%w: %q", ErrInvalidProductOptionInputType, string(input.InputType))
	}
	if strings.TrimSpace(input.Title) == "" {
		return ProductOption{}, fmt.Errorf("title is required")
	}
	parent, err := s.repo.GetProductByID(ctx, input.ProductID)
	if err != nil {
		return ProductOption{}, err
	}
	if parent.Kind != KindSimple && parent.Kind != KindConfigurable {
		return ProductOption{}, fmt.Errorf("%w: product %q is kind %q", ErrOptionInputKindNotPurchasable, parent.SKU, string(parent.Kind))
	}
	if strings.TrimSpace(input.Locale) == "" {
		return ProductOption{}, fmt.Errorf("locale is required")
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && input.Locale != def {
		return ProductOption{}, fmt.Errorf("%w: got %q, want %q", ErrLocaleMustBeDefault, input.Locale, def)
	}
	if err := validateOptionPriceShape(input.InputType, input.PriceValue, input.PriceType); err != nil {
		return ProductOption{}, err
	}
	if err := validateOptionConstraintShape(input.InputType, input.MaxChars, input.MaxFileBytes, input.FileExtensions); err != nil {
		return ProductOption{}, err
	}
	if input.VisibleIf != nil {
		siblings, serr := s.optionsWithValues(ctx, input.ProductID)
		if serr != nil {
			return ProductOption{}, serr
		}
		if err := validateOptionVisibility(input.VisibleIf, input.SortOrder, uuid.Nil, siblings); err != nil {
			return ProductOption{}, err
		}
	}
	return s.repo.CreateProductOption(ctx, input)
}

// UpdateProductOption applies pointer-style "no change = nil" updates - Input-type transitions are allowed but constrained: changing from a select-family type to a non-select type or vice-versa is rejected when the option already has values (would leave orphan rows / mismatched shape)
func (s *optionService) UpdateProductOption(ctx context.Context, input UpdateProductOptionInput) (ProductOption, error) {
	current, err := s.repo.GetProductOptionByID(ctx, input.ID)
	if err != nil {
		return ProductOption{}, err
	}
	finalInputType := current.InputType
	if input.InputType != nil {
		if !input.InputType.IsKnown() {
			return ProductOption{}, fmt.Errorf("%w: %q", ErrInvalidProductOptionInputType, string(*input.InputType))
		}
		finalInputType = *input.InputType
	}
	if finalInputType.IsSelect() != current.InputType.IsSelect() {
		existing, err := s.repo.ListProductOptionValuesByOptionID(ctx, current.ID)
		if err != nil {
			return ProductOption{}, err
		}
		if len(existing) > 0 {
			return ProductOption{}, fmt.Errorf("%w: option has %d value rows incompatible with new input type %q", ErrInvalidProductOptionInputType, len(existing), string(finalInputType))
		}
	}
	// multiselect -> dropdown/radio keeps its values, but more than one default would violate the single-default rule the create path enforces
	if finalInputType.IsSingleSelect() && !current.InputType.IsSingleSelect() && current.InputType.IsSelect() {
		n, err := s.repo.CountProductOptionValueDefaultsForOption(ctx, current.ID)
		if err != nil {
			return ProductOption{}, err
		}
		if n > 1 {
			return ProductOption{}, fmt.Errorf("%w: %d values are marked default - unset all but one first", ErrOptionMultiDefaultOnSingle, n)
		}
	}

	finalRequired := current.IsRequired
	if input.IsRequired != nil {
		finalRequired = *input.IsRequired
	}
	// A required select with zero values can never validate at add-to-cart - the product would be silently unpurchasable
	if finalRequired && !current.IsRequired && finalInputType.IsSelect() {
		existing, err := s.repo.ListProductOptionValuesByOptionID(ctx, current.ID)
		if err != nil {
			return ProductOption{}, err
		}
		if len(existing) == 0 {
			return ProductOption{}, fmt.Errorf("%w: add its values first", ErrOptionRequiredNeedsValues)
		}
	}
	finalSortOrder := current.SortOrder
	if input.SortOrder != nil {
		finalSortOrder = *input.SortOrder
	}
	// Price and constraint fields are tri-state like VisibleIf - a GraphQL null decodes to a nil pointer ("leave unchanged"), so clearing needs its own flag or an emptied admin field becomes a silent no-op
	finalPriceValue := current.PriceValue
	finalPriceType := current.PriceType
	if input.ClearPrice {
		finalPriceValue = nil
		finalPriceType = nil
	} else {
		if input.PriceValue != nil {
			v := *input.PriceValue
			finalPriceValue = &v
		}
		if input.PriceType != nil {
			t := *input.PriceType
			finalPriceType = &t
		}
	}
	finalMaxChars := current.MaxChars
	if input.ClearMaxChars {
		finalMaxChars = nil
	} else if input.MaxChars != nil {
		v := *input.MaxChars
		finalMaxChars = &v
	}
	finalMaxFileBytes := current.MaxFileBytes
	if input.ClearMaxFileBytes {
		finalMaxFileBytes = nil
	} else if input.MaxFileBytes != nil {
		v := *input.MaxFileBytes
		finalMaxFileBytes = &v
	}
	finalFileExtensions := current.FileExtensions
	if input.ClearFileExtensions {
		finalFileExtensions = ""
	} else if input.FileExtensions != nil {
		finalFileExtensions = *input.FileExtensions
	}

	// After the transition, re-validate shape against the final input type
	if err := validateOptionPriceShape(finalInputType, finalPriceValue, finalPriceType); err != nil {
		return ProductOption{}, err
	}
	if err := validateOptionConstraintShape(finalInputType, finalMaxChars, finalMaxFileBytes, finalFileExtensions); err != nil {
		return ProductOption{}, err
	}

	finalVisibleIf := current.VisibleIf
	if input.ClearVisibleIf {
		finalVisibleIf = nil
	} else if input.VisibleIf != nil {
		finalVisibleIf = input.VisibleIf
	}
	siblings, err := s.optionsWithValues(ctx, current.ProductID)
	if err != nil {
		return ProductOption{}, err
	}
	if finalVisibleIf != nil {
		if err := validateOptionVisibility(finalVisibleIf, finalSortOrder, current.ID, siblings); err != nil {
			return ProductOption{}, err
		}
	}
	// A sort_order move can break SIBLING conditions that reference this option (they require strictly lower parent order) - re-check them against the new ordering before committing
	if finalSortOrder != current.SortOrder {
		for _, sib := range siblings {
			if sib.ID == current.ID || sib.VisibleIf == nil {
				continue
			}
			adjusted := make([]ProductOption, len(siblings))
			copy(adjusted, siblings)
			for i := range adjusted {
				if adjusted[i].ID == current.ID {
					adjusted[i].SortOrder = finalSortOrder
				}
			}
			if err := validateOptionVisibility(sib.VisibleIf, sib.SortOrder, sib.ID, adjusted); err != nil {
				return ProductOption{}, fmt.Errorf("reorder would break the visibility rule on option %q: %w", sib.Title, err)
			}
		}
	}
	return s.repo.UpdateProductOption(ctx, current.ID, UpdateProductOptionFields{
		InputType:      finalInputType,
		IsRequired:     finalRequired,
		SortOrder:      finalSortOrder,
		PriceValue:     finalPriceValue,
		PriceType:      finalPriceType,
		MaxChars:       finalMaxChars,
		MaxFileBytes:   finalMaxFileBytes,
		FileExtensions: finalFileExtensions,
		VisibleIf:      finalVisibleIf,
	})
}

// DeleteProductOption removes an option and cascades its content + values (FK CASCADE handles those)
func (s *optionService) DeleteProductOption(ctx context.Context, id uuid.UUID) error {
	current, err := s.repo.GetProductOptionByID(ctx, id)
	if err != nil {
		return err
	}
	// A sibling's visibility rule referencing this option would dangle - make the operator remove the dependency first
	siblings, err := s.optionsWithValues(ctx, current.ProductID)
	if err != nil {
		return err
	}
	for _, sib := range siblings {
		if sib.ID != id && groupReferencesOption(sib.VisibleIf, id) {
			return fmt.Errorf("%w: option %q depends on this one - remove its visibility rule first", ErrOptionVisibilityInvalid, sib.Title)
		}
	}
	return s.repo.DeleteProductOption(ctx, id)
}

// DuplicateProductOption clones an option (with all its content rows and, for select-family types, all its values + their content rows) onto the same parent product
// The clone lands at sort_order = max+1 so it doesn't collide with the source - Returns the new option's id
func (s *optionService) DuplicateProductOption(ctx context.Context, id uuid.UUID) (ProductOption, error) {
	source, err := s.repo.GetProductOptionByID(ctx, id)
	if err != nil {
		return ProductOption{}, err
	}
	sourceContent, err := s.repo.ListProductOptionContent(ctx, source.ID)
	if err != nil {
		return ProductOption{}, err
	}
	if len(sourceContent) == 0 {
		return ProductOption{}, fmt.Errorf("source option %s has no content rows", source.ID)
	}
	def := s.resolveDefaultLocale(ctx)
	// Find default-locale row for the initial create - fall back to first
	var initial ProductOptionContent
	initial = sourceContent[0]
	for _, c := range sourceContent {
		if c.Locale == def {
			initial = c
			break
		}
	}
	// Sort order: max existing + 1, computed against the current option set
	existing, err := s.repo.ListProductOptionsByProductID(ctx, source.ProductID)
	if err != nil {
		return ProductOption{}, err
	}
	nextSort := int32(0)
	for _, o := range existing {
		if o.SortOrder >= nextSort {
			nextSort = o.SortOrder + 1
		}
	}

	created, err := s.repo.CreateProductOption(ctx, AddProductOptionInput{
		ProductID:      source.ProductID,
		InputType:      source.InputType,
		IsRequired:     source.IsRequired,
		SortOrder:      nextSort,
		PriceValue:     source.PriceValue,
		PriceType:      source.PriceType,
		MaxChars:       source.MaxChars,
		MaxFileBytes:   source.MaxFileBytes,
		FileExtensions: source.FileExtensions,
		Locale:         initial.Locale,
		Title:          initial.Title,
		HelpText:       initial.HelpText,
		VisibleIf:      source.VisibleIf,
	})
	if err != nil {
		return ProductOption{}, err
	}

	// Copy every non-default-locale content row (the default-locale row landed via CreateProductOption above)
	for _, c := range sourceContent {
		if c.Locale == initial.Locale {
			continue
		}
		if _, err := s.repo.UpsertProductOptionContent(ctx, UpsertProductOptionContentInput{
			OptionID: created.ID,
			Locale:   c.Locale,
			Title:    c.Title,
			HelpText: c.HelpText,
		}); err != nil {
			return ProductOption{}, fmt.Errorf("clone option content %q: %w", c.Locale, err)
		}
	}

	// Copy values + their content for select-family options
	if source.InputType.IsSelect() {
		sourceValues, err := s.repo.ListProductOptionValuesByOptionID(ctx, source.ID)
		if err != nil {
			return ProductOption{}, err
		}
		// Batch-load all value content up front (was an N+1 per-value loop)
		valueIDs := make([]uuid.UUID, len(sourceValues))
		for i, v := range sourceValues {
			valueIDs[i] = v.ID
		}
		vcByValue := map[uuid.UUID][]ProductOptionValueContent{}
		if len(valueIDs) > 0 {
			vcByValue, err = s.repo.ListProductOptionValueContentByValueIDs(ctx, valueIDs)
			if err != nil {
				return ProductOption{}, err
			}
		}
		for _, v := range sourceValues {
			vc := vcByValue[v.ID]
			if len(vc) == 0 {
				continue // shouldn't happen but skip cleanly
			}
			var vInitial ProductOptionValueContent
			vInitial = vc[0]
			for _, c := range vc {
				if c.Locale == def {
					vInitial = c
					break
				}
			}
			newValue, err := s.repo.CreateProductOptionValue(ctx, AddProductOptionValueInput{
				OptionID:        created.ID,
				SortOrder:       v.SortOrder,
				PriceValue:      v.PriceValue,
				PriceType:       v.PriceType,
				IsDefault:       v.IsDefault,
				Quantity:        v.Quantity,
				ForcedLineQty:   v.ForcedLineQty,
				SourceProductID: v.SourceProductID,
				Locale:          vInitial.Locale,
				Title:           vInitial.Title,
			})
			if err != nil {
				return ProductOption{}, fmt.Errorf("clone option value: %w", err)
			}
			for _, c := range vc {
				if c.Locale == vInitial.Locale {
					continue
				}
				if _, err := s.repo.UpsertProductOptionValueContent(ctx, UpsertProductOptionValueContentInput{
					ValueID: newValue.ID,
					Locale:  c.Locale,
					Title:   c.Title,
				}); err != nil {
					return ProductOption{}, fmt.Errorf("clone option value content %q: %w", c.Locale, err)
				}
			}
		}
	}

	return s.repo.GetProductOptionByID(ctx, created.ID)
}

func (s *optionService) UpsertProductOptionContent(ctx context.Context, input UpsertProductOptionContentInput) (ProductOptionContent, error) {
	if strings.TrimSpace(input.Locale) == "" {
		return ProductOptionContent{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(input.Title) == "" {
		return ProductOptionContent{}, fmt.Errorf("title is required")
	}
	// help_text is optional and may be the empty string
	if _, err := s.repo.GetProductOptionByID(ctx, input.OptionID); err != nil {
		return ProductOptionContent{}, err
	}
	return s.repo.UpsertProductOptionContent(ctx, input)
}

// DeleteProductOptionContent: removing the default-locale translation while other translations still exist is rejected - same rule as ProductContent / BundleOptionContent
func (s *optionService) DeleteProductOptionContent(ctx context.Context, optionID uuid.UUID, locale string) error {
	def := s.resolveDefaultLocale(ctx)
	if def != "" && locale == def {
		others, err := s.repo.ListProductOptionContent(ctx, optionID)
		if err != nil {
			return err
		}
		hasTranslations := false
		for _, c := range others {
			if c.Locale != def {
				hasTranslations = true
				break
			}
		}
		if hasTranslations {
			return ErrCannotDeleteDefaultLocale
		}
	}
	return s.repo.DeleteProductOptionContent(ctx, optionID, locale)
}

// ==================== Option values ====================

// AddProductOptionValue adds one pre-defined choice to a select-family option (dropdown/radio/checkbox/multiselect) - Single-select options reject a second default; multi-select options allow zero or more - When SourceProductID is set, the referenced product must exist and be of kind SIMPLE, and PriceType must be FIXED - see validateValueSourceProduct
func (s *optionService) AddProductOptionValue(ctx context.Context, input AddProductOptionValueInput) (ProductOptionValue, error) {
	if strings.TrimSpace(input.Title) == "" {
		return ProductOptionValue{}, fmt.Errorf("title is required")
	}
	if err := validateForcedLineQty(input.ForcedLineQty); err != nil {
		return ProductOptionValue{}, err
	}
	if !input.PriceType.IsKnown() {
		return ProductOptionValue{}, fmt.Errorf("%w: %q", ErrInvalidProductOptionPriceType, string(input.PriceType))
	}
	if err := validatePercentRange(input.PriceType, input.PriceValue); err != nil {
		return ProductOptionValue{}, err
	}
	option, err := s.repo.GetProductOptionByID(ctx, input.OptionID)
	if err != nil {
		return ProductOptionValue{}, err
	}
	if !option.InputType.IsSelect() {
		return ProductOptionValue{}, fmt.Errorf("%w: option is %q", ErrOptionValueOnNonSelect, string(option.InputType))
	}
	if strings.TrimSpace(input.Locale) == "" {
		return ProductOptionValue{}, fmt.Errorf("locale is required")
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && input.Locale != def {
		return ProductOptionValue{}, fmt.Errorf("%w: got %q, want %q", ErrLocaleMustBeDefault, input.Locale, def)
	}
	if input.IsDefault && option.InputType.IsSingleSelect() {
		n, err := s.repo.CountProductOptionValueDefaultsForOption(ctx, option.ID)
		if err != nil {
			return ProductOptionValue{}, err
		}
		if n > 0 {
			return ProductOptionValue{}, fmt.Errorf("%w: option already has a default value", ErrOptionMultiDefaultOnSingle)
		}
	}
	if err := s.validateValueSourceProduct(ctx, input.SourceProductID, input.PriceType); err != nil {
		return ProductOptionValue{}, err
	}
	return s.repo.CreateProductOptionValue(ctx, input)
}

// validateValueSourceProduct enforces: source product (if set) exists and is KindSimple; PriceType is FIXED whenever source is set
func (s *optionService) validateValueSourceProduct(ctx context.Context, sourceID *uuid.UUID, priceType ProductOptionPriceType) error {
	if sourceID == nil || *sourceID == (uuid.UUID{}) {
		return nil
	}
	if priceType == OptionPricePercent {
		return ErrOptionValuePercentOnLinked
	}
	src, err := s.repo.GetProductByID(ctx, *sourceID)
	if err != nil {
		return err
	}
	if src.Kind != KindSimple {
		return fmt.Errorf("%w: product %q is kind %q", ErrOptionValueSourceNotSimple, src.SKU, string(src.Kind))
	}
	return nil
}

func (s *optionService) UpdateProductOptionValue(ctx context.Context, input UpdateProductOptionValueInput) (ProductOptionValue, error) {
	current, err := s.repo.GetProductOptionValueByID(ctx, input.ID)
	if err != nil {
		return ProductOptionValue{}, err
	}
	finalSortOrder := current.SortOrder
	if input.SortOrder != nil {
		finalSortOrder = *input.SortOrder
	}
	finalPriceValue := current.PriceValue
	if input.PriceValue != nil {
		finalPriceValue = *input.PriceValue
	}
	finalPriceType := current.PriceType
	if input.PriceType != nil {
		if !input.PriceType.IsKnown() {
			return ProductOptionValue{}, fmt.Errorf("%w: %q", ErrInvalidProductOptionPriceType, string(*input.PriceType))
		}
		finalPriceType = *input.PriceType
	}
	finalIsDefault := current.IsDefault
	if input.IsDefault != nil {
		finalIsDefault = *input.IsDefault
	}
	finalQuantity := current.Quantity
	if input.Quantity != nil {
		if *input.Quantity < 1 {
			return ProductOptionValue{}, fmt.Errorf("quantity must be positive")
		}
		finalQuantity = *input.Quantity
	}
	// Forced line qty: pointer sets; zero-or-negative clears; nil = no change
	finalForcedLineQty := current.ForcedLineQty
	if input.ForcedLineQty != nil {
		if *input.ForcedLineQty <= 0 {
			finalForcedLineQty = nil
		} else {
			if err := validateForcedLineQty(input.ForcedLineQty); err != nil {
				return ProductOptionValue{}, err
			}
			q := *input.ForcedLineQty
			finalForcedLineQty = &q
		}
	}
	// Source product: pointer-to-uuid sets/replaces; pointer-to-zero unlinks; nil leaves untouched
	finalSourceProductID := current.SourceProductID
	if input.SourceProductID != nil {
		if *input.SourceProductID == (uuid.UUID{}) {
			finalSourceProductID = nil
		} else {
			id := *input.SourceProductID
			finalSourceProductID = &id
		}
	}
	if err := s.validateValueSourceProduct(ctx, finalSourceProductID, finalPriceType); err != nil {
		return ProductOptionValue{}, err
	}
	if err := validatePercentRange(finalPriceType, finalPriceValue); err != nil {
		return ProductOptionValue{}, err
	}

	if finalIsDefault && !current.IsDefault {
		option, err := s.repo.GetProductOptionByID(ctx, current.OptionID)
		if err != nil {
			return ProductOptionValue{}, err
		}
		if option.InputType.IsSingleSelect() {
			n, err := s.repo.CountProductOptionValueDefaultsForOption(ctx, option.ID)
			if err != nil {
				return ProductOptionValue{}, err
			}
			if n > 0 {
				return ProductOptionValue{}, fmt.Errorf("%w: option already has a default value", ErrOptionMultiDefaultOnSingle)
			}
		}
	}

	return s.repo.UpdateProductOptionValue(ctx, current.ID, finalSortOrder, finalPriceValue, finalPriceType, finalIsDefault, finalSourceProductID, finalQuantity, finalForcedLineQty)
}

func (s *optionService) DeleteProductOptionValue(ctx context.Context, id uuid.UUID) error {
	current, err := s.repo.GetProductOptionValueByID(ctx, id)
	if err != nil {
		return err
	}
	option, err := s.repo.GetProductOptionByID(ctx, current.OptionID)
	if err != nil {
		return err
	}
	// A sibling's visibility rule pinned to this value would silently degrade - anyOf could never match again, noneOf would become always-true - same guard as option delete
	siblings, err := s.optionsWithValues(ctx, option.ProductID)
	if err != nil {
		return err
	}
	for _, sib := range siblings {
		if sib.ID != option.ID && groupReferencesValue(sib.VisibleIf, id) {
			return fmt.Errorf("%w: the visibility rule on option %q references this value - remove it first", ErrOptionVisibilityInvalid, sib.Title)
		}
	}
	// Removing the last value of a required select leaves an option that can never validate, silently bricking purchase
	if option.IsRequired && option.InputType.IsSelect() {
		vals, verr := s.repo.ListProductOptionValuesByOptionID(ctx, option.ID)
		if verr != nil {
			return verr
		}
		if len(vals) <= 1 {
			return fmt.Errorf("%w: make the option optional before removing its last value", ErrOptionRequiredNeedsValues)
		}
	}
	return s.repo.DeleteProductOptionValue(ctx, id)
}

func (s *optionService) UpsertProductOptionValueContent(ctx context.Context, input UpsertProductOptionValueContentInput) (ProductOptionValueContent, error) {
	if strings.TrimSpace(input.Locale) == "" {
		return ProductOptionValueContent{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(input.Title) == "" {
		return ProductOptionValueContent{}, fmt.Errorf("title is required")
	}
	if _, err := s.repo.GetProductOptionValueByID(ctx, input.ValueID); err != nil {
		return ProductOptionValueContent{}, err
	}
	return s.repo.UpsertProductOptionValueContent(ctx, input)
}

func (s *optionService) DeleteProductOptionValueContent(ctx context.Context, valueID uuid.UUID, locale string) error {
	def := s.resolveDefaultLocale(ctx)
	if def != "" && locale == def {
		others, err := s.repo.ListProductOptionValueContent(ctx, valueID)
		if err != nil {
			return err
		}
		hasTranslations := false
		for _, c := range others {
			if c.Locale != def {
				hasTranslations = true
				break
			}
		}
		if hasTranslations {
			return ErrCannotDeleteDefaultLocale
		}
	}
	return s.repo.DeleteProductOptionValueContent(ctx, valueID, locale)
}

// ==================== Read-side: full options view ====================

// GetProductOptions loads every option for a product, hydrating each with its values (for select-family) and resolved titles (request locale → default-locale fallback) - Mirrors GetBundleOptions
func (s *optionService) GetProductOptions(ctx context.Context, productID uuid.UUID, locale string) ([]ProductOption, error) {
	options, err := s.repo.ListProductOptionsByProductID(ctx, productID)
	if err != nil {
		return nil, err
	}
	if len(options) == 0 {
		return []ProductOption{}, nil
	}

	defLocale := s.resolveDefaultLocale(ctx)
	if strings.TrimSpace(locale) == "" {
		locale = defLocale
	}

	optionIDs := make([]uuid.UUID, len(options))
	for i, o := range options {
		optionIDs[i] = o.ID
	}

	contentByOption, err := s.repo.ListProductOptionContentByOptionIDs(ctx, optionIDs)
	if err != nil {
		return nil, err
	}
	valuesByOption, err := s.repo.ListProductOptionValuesByOptionIDs(ctx, optionIDs)
	if err != nil {
		return nil, err
	}

	// Collect every value ID to batch-load value content
	var valueIDs []uuid.UUID
	// Collect referenced source product ids too - populated only on values that link to an existing SIMPLE product
	var sourceProductIDs []uuid.UUID
	seenSources := make(map[uuid.UUID]struct{})
	for _, vals := range valuesByOption {
		for _, v := range vals {
			valueIDs = append(valueIDs, v.ID)
			if v.SourceProductID != nil {
				if _, ok := seenSources[*v.SourceProductID]; !ok {
					seenSources[*v.SourceProductID] = struct{}{}
					sourceProductIDs = append(sourceProductIDs, *v.SourceProductID)
				}
			}
		}
	}
	valueContentByValue := map[uuid.UUID][]ProductOptionValueContent{}
	if len(valueIDs) > 0 {
		valueContentByValue, err = s.repo.ListProductOptionValueContentByValueIDs(ctx, valueIDs)
		if err != nil {
			return nil, err
		}
	}

	// Load referenced products + their content (locale fallback)
	sourceByID := make(map[uuid.UUID]Product, len(sourceProductIDs))
	sourceContentByID := map[uuid.UUID]ProductContent{}
	if len(sourceProductIDs) > 0 {
		// Batch-load the referenced source products (was an N+1 GetProductByID loop) - A since-deleted source product simply
		// drops out of the map and is handled by the ok-check below rather than as a hard error
		sources, err := s.repo.ListProductsWithAttributesByIDs(ctx, sourceProductIDs)
		if err != nil {
			return nil, fmt.Errorf("load option value source products: %w", err)
		}
		for _, p := range sources {
			sourceByID[p.ID] = p
		}
		sourceContentByID, err = s.repo.ListProductContentByScope(ctx, sourceProductIDs, locale)
		if err != nil {
			return nil, fmt.Errorf("load option value source content: %w", err)
		}
		if locale != defLocale && defLocale != "" {
			fallback, err := s.repo.ListProductContentByScope(ctx, sourceProductIDs, defLocale)
			if err != nil {
				return nil, fmt.Errorf("load option value source fallback content: %w", err)
			}
			for pid, c := range fallback {
				if _, ok := sourceContentByID[pid]; !ok {
					sourceContentByID[pid] = c
				}
			}
		}
	}

	for i := range options {
		opt := &options[i]
		opt.AllContent = contentByOption[opt.ID]
		opt.Title, opt.HelpText = resolveProductOptionContent(opt.AllContent, locale, defLocale)

		vals := valuesByOption[opt.ID]
		for j := range vals {
			v := &vals[j]
			v.AllContent = valueContentByValue[v.ID]
			v.Title = resolveProductOptionValueTitle(v.AllContent, locale, defLocale)
			if v.SourceProductID != nil {
				if p, ok := sourceByID[*v.SourceProductID]; ok {
					v.SourceProductURL = p.URL
					v.SourceProductSKU = p.SKU
					v.SourceProductDefaultPrice = p.DefaultPrice
					v.SourceProductCurrency = p.DefaultCurrencyCode
					if c, ok := sourceContentByID[p.ID]; ok {
						v.SourceProductName = c.Name
					}
				}
			}
		}
		opt.Values = vals
	}

	return options, nil
}

// resolveProductOptionContent returns the resolved (title, help_text) pair for an option, picking the requested locale
// first then falling back to default-locale then the first available row
func resolveProductOptionContent(all []ProductOptionContent, locale, defLocale string) (string, string) {
	for _, c := range all {
		if c.Locale == locale {
			return c.Title, c.HelpText
		}
	}
	if defLocale != "" && defLocale != locale {
		for _, c := range all {
			if c.Locale == defLocale {
				return c.Title, c.HelpText
			}
		}
	}
	if len(all) > 0 {
		return all[0].Title, all[0].HelpText
	}
	return "", ""
}

func resolveProductOptionValueTitle(all []ProductOptionValueContent, locale, defLocale string) string {
	for _, c := range all {
		if c.Locale == locale {
			return c.Title
		}
	}
	if defLocale != "" && defLocale != locale {
		for _, c := range all {
			if c.Locale == defLocale {
				return c.Title
			}
		}
	}
	if len(all) > 0 {
		return all[0].Title
	}
	return ""
}

// --- shape validators ---

func validateOptionPriceShape(t ProductOptionInputType, priceValue *int32, priceType *ProductOptionPriceType) error {
	if t.IsSelect() {
		if priceValue != nil || priceType != nil {
			return ErrOptionPriceOnSelect
		}
		return nil
	}
	// Non-select: price columns either both set or both unset
	if (priceValue == nil) != (priceType == nil) {
		return fmt.Errorf("price_value and price_type must be set together")
	}
	if priceType != nil && !priceType.IsKnown() {
		return fmt.Errorf("%w: %q", ErrInvalidProductOptionPriceType, string(*priceType))
	}
	if priceType != nil && priceValue != nil {
		if err := validatePercentRange(*priceType, *priceValue); err != nil {
			return err
		}
	}
	return nil
}

// A percent delta outside [-100, 1000] is a typo, not a price - below -100 the line goes negative by definition, and the cart clamps but shouldn't have to
func validatePercentRange(t ProductOptionPriceType, v int32) error {
	if t != OptionPricePercent {
		return nil
	}
	if v < -100 || v > 1000 {
		return fmt.Errorf("%w: percent adjustment must be between -100 and 1000, got %d", ErrInvalidProductOptionPriceType, v)
	}
	return nil
}

// Forced line quantities multiply into order lines - fractional forces would make "Set of 4" ship 2.5 units
func validateForcedLineQty(q *float64) error {
	if q == nil {
		return nil
	}
	if *q <= 0 {
		return fmt.Errorf("forced line quantity must be positive")
	}
	if *q != math.Trunc(*q) {
		return fmt.Errorf("forced line quantity must be a whole number, got %v", *q)
	}
	return nil
}

func validateOptionConstraintShape(t ProductOptionInputType, maxChars *int32, maxFileBytes *int64, fileExtensions string) error {
	if maxChars != nil && !t.IsText() {
		return ErrOptionTextConstraintsOnNonText
	}
	if (maxFileBytes != nil || fileExtensions != "") && !t.IsFile() {
		return ErrOptionFileConstraintsOnNonFile
	}
	return nil
}

// optionsWithValues lists a product's options with Values attached - visibility validation needs the parent value sets,
// which the bare repository list does not hydrate
func (s *optionService) optionsWithValues(ctx context.Context, productID uuid.UUID) ([]ProductOption, error) {
	options, err := s.repo.ListProductOptionsByProductID(ctx, productID)
	if err != nil {
		return nil, err
	}
	if len(options) == 0 {
		return options, nil
	}
	ids := make([]uuid.UUID, len(options))
	for i, o := range options {
		ids[i] = o.ID
	}
	valuesByOption, err := s.repo.ListProductOptionValuesByOptionIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	contentByOption, err := s.repo.ListProductOptionContentByOptionIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	defLocale := s.resolveDefaultLocale(ctx)
	for i := range options {
		options[i].Values = valuesByOption[options[i].ID]
		// Resolve a display title (default locale, else any) so
		// validation errors can name the option
		if rows := contentByOption[options[i].ID]; len(rows) > 0 {
			options[i].Title = rows[0].Title
			for _, c := range rows {
				if c.Locale == defLocale {
					options[i].Title = c.Title
					break
				}
			}
		}
	}
	return options, nil
}

// validateOptionVisibility structurally checks a visibility condition: known ops/logic, no self-reference, parents exist on the product with STRICTLY lower sort_order (canonical order (sort_order, created_at) - the strict-order rule is what makes evaluation a cycle-free single pass), anyOf/noneOf only on select-family parents with value_ids that belong to them
func validateOptionVisibility(g *OptionConditionGroup, ownOrder int32, ownID uuid.UUID, siblings []ProductOption) error {
	if g == nil {
		return nil
	}
	if g.Logic != "" && g.Logic != "and" && g.Logic != "or" {
		return fmt.Errorf("%w: logic must be \"and\" or \"or\"", ErrOptionVisibilityInvalid)
	}
	if len(g.Rules) == 0 && len(g.Groups) == 0 {
		return fmt.Errorf("%w: a group needs at least one rule", ErrOptionVisibilityInvalid)
	}
	byID := make(map[uuid.UUID]ProductOption, len(siblings))
	for _, sib := range siblings {
		byID[sib.ID] = sib
	}
	for _, r := range g.Rules {
		if !OptionRuleOps[r.Op] {
			return fmt.Errorf("%w: unknown op %q", ErrOptionVisibilityInvalid, r.Op)
		}
		if r.OptionID == ownID {
			return fmt.Errorf("%w: an option cannot depend on itself", ErrOptionVisibilityInvalid)
		}
		parent, ok := byID[r.OptionID]
		if !ok {
			return fmt.Errorf("%w: rule references an option not on this product", ErrOptionVisibilityInvalid)
		}
		if parent.SortOrder >= ownOrder {
			return fmt.Errorf("%w: may only depend on options ordered BEFORE this one - move %q above it first", ErrOptionVisibilityInvalid, parent.Title)
		}
		if r.Op == "anyOf" || r.Op == "noneOf" {
			if !parent.InputType.IsSelect() {
				return fmt.Errorf("%w: op %q needs a choice-type parent option (%q is %s)", ErrOptionVisibilityInvalid, r.Op, parent.Title, parent.InputType)
			}
			if len(r.ValueIDs) == 0 {
				return fmt.Errorf("%w: op %q needs at least one value", ErrOptionVisibilityInvalid, r.Op)
			}
			valid := make(map[uuid.UUID]bool, len(parent.Values))
			for _, v := range parent.Values {
				valid[v.ID] = true
			}
			for _, vid := range r.ValueIDs {
				if !valid[vid] {
					return fmt.Errorf("%w: rule references a value that is not on option %q", ErrOptionVisibilityInvalid, parent.Title)
				}
			}
		}
	}
	for i := range g.Groups {
		if err := validateOptionVisibility(&g.Groups[i], ownOrder, ownID, siblings); err != nil {
			return err
		}
	}
	return nil
}

// groupReferencesValue reports whether any rule in the tree pins value id
func groupReferencesValue(g *OptionConditionGroup, id uuid.UUID) bool {
	if g == nil {
		return false
	}
	for _, r := range g.Rules {
		if slices.Contains(r.ValueIDs, id) {
			return true
		}
	}
	for i := range g.Groups {
		if groupReferencesValue(&g.Groups[i], id) {
			return true
		}
	}
	return false
}

// groupReferencesOption reports whether any rule in the tree targets id
func groupReferencesOption(g *OptionConditionGroup, id uuid.UUID) bool {
	if g == nil {
		return false
	}
	for _, r := range g.Rules {
		if r.OptionID == id {
			return true
		}
	}
	for i := range g.Groups {
		if groupReferencesOption(&g.Groups[i], id) {
			return true
		}
	}
	return false
}
