package catalog

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
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
		return ProductOption{}, ErrTitleRequired
	}
	owner := OptionOwner{ProductID: input.ProductID, TemplateID: input.TemplateID}
	if (owner.ProductID == uuid.Nil) == (owner.TemplateID == uuid.Nil) {
		return ProductOption{}, fmt.Errorf("%w: an option belongs to one product or one template", ErrOptionTemplateInvalid)
	}
	if owner.IsTemplate() {
		if _, err := s.repo.GetOptionTemplate(ctx, owner.TemplateID); err != nil {
			return ProductOption{}, err
		}
	} else if err := s.checkTemplateTarget(ctx, owner.ProductID); err != nil {
		return ProductOption{}, err
	}
	if strings.TrimSpace(input.Locale) == "" {
		return ProductOption{}, ErrLocaleRequired
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
	if err := s.checkOptionImage(ctx, input.ImageAssetID); err != nil {
		return ProductOption{}, err
	}
	if input.VisibleIf != nil {
		siblings, serr := s.ownerOptions(ctx, owner)
		if serr != nil {
			return ProductOption{}, serr
		}
		if err := validateOptionVisibility(input.VisibleIf, input.SortOrder, uuid.Nil, siblings); err != nil {
			return ProductOption{}, err
		}
	}
	created, err := s.repo.CreateProductOption(ctx, input)
	return created, s.announced(ctx, owner, err)
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
	// A required select with zero values can never validate at add-to-cart - checked on the final state, so a field turning into a required dropdown counts too
	if finalRequired && finalInputType.IsSelect() {
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

	finalImage, err := s.nextOptionImage(ctx, current.ImageAssetID, input.ImageAssetID)
	if err != nil {
		return ProductOption{}, err
	}
	finalVisibleIf := current.VisibleIf
	if input.ClearVisibleIf {
		finalVisibleIf = nil
	} else if input.VisibleIf != nil {
		finalVisibleIf = input.VisibleIf
	}
	siblings, err := s.ownerOptions(ctx, current.Owner())
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
		adjusted := make([]ProductOption, len(siblings))
		copy(adjusted, siblings)
		for i := range adjusted {
			if adjusted[i].ID == current.ID {
				adjusted[i].SortOrder = finalSortOrder
			}
		}
		for _, sib := range siblings {
			if sib.ID == current.ID || sib.VisibleIf == nil || sib.TemplateID != current.TemplateID {
				continue
			}
			if err := validateOptionVisibility(sib.VisibleIf, sib.SortOrder, sib.ID, adjusted); err != nil {
				return ProductOption{}, fmt.Errorf("reorder would break the visibility rule on option %q: %w", sib.Title, err)
			}
		}
	}
	updated, err := s.repo.UpdateProductOption(ctx, current.ID, UpdateProductOptionFields{
		InputType:      finalInputType,
		IsRequired:     finalRequired,
		SortOrder:      finalSortOrder,
		PriceValue:     finalPriceValue,
		PriceType:      finalPriceType,
		MaxChars:       finalMaxChars,
		MaxFileBytes:   finalMaxFileBytes,
		FileExtensions: finalFileExtensions,
		VisibleIf:      finalVisibleIf,
		ImageAssetID:   finalImage,
	})
	return updated, s.announced(ctx, current.Owner(), err)
}

// DeleteProductOption removes an option and cascades its content + values (FK CASCADE handles those)
func (s *optionService) DeleteProductOption(ctx context.Context, id uuid.UUID) error {
	current, err := s.repo.GetProductOptionByID(ctx, id)
	if err != nil {
		return err
	}
	// A sibling's visibility rule referencing this option would dangle - make the operator remove the dependency first
	siblings, err := s.ownerOptions(ctx, current.Owner())
	if err != nil {
		return err
	}
	for _, sib := range siblings {
		if sib.ID != id && sib.TemplateID == current.TemplateID && groupReferencesOption(sib.VisibleIf, id) {
			return fmt.Errorf("%w: option %q depends on this one - remove its visibility rule first", ErrOptionVisibilityInvalid, sib.Title)
		}
	}
	if rule, err := s.ruleUsing(ctx, current.Owner(), &id, nil); err != nil || rule != nil {
		if err != nil {
			return err
		}
		return ruleInUseError(rule)
	}
	if current.TemplateID != uuid.Nil {
		if current.Values, err = s.repo.ListProductOptionValuesByOptionID(ctx, id); err != nil {
			return err
		}
		users, err := s.repo.ListAllProductIDsUsingOptionTemplate(ctx, current.TemplateID)
		if err != nil {
			return err
		}
		if err := s.refuseProductLeaning(ctx, users, current); err != nil {
			return err
		}
	}
	return s.announced(ctx, current.Owner(), s.repo.DeleteProductOption(ctx, id))
}

// DuplicateProductOption clones an option (with all its content rows and, for select-family types, all its values + their content rows) onto the same parent product
// The clone lands at sort_order = max+1 so it doesn't collide with the source - Returns the new option's id
func (s *optionService) DuplicateProductOption(ctx context.Context, id uuid.UUID) (ProductOption, error) {
	def := s.resolveDefaultLocale(ctx)
	var out ProductOption
	err := s.inTx(ctx, func(r Repository) error {
		var err error
		out, err = duplicateProductOption(ctx, r, id, def)
		return err
	})
	return out, s.announced(ctx, out.Owner(), err)
}

func duplicateProductOption(ctx context.Context, r OptionRepository, id uuid.UUID, def string) (ProductOption, error) {
	source, err := r.GetProductOptionByID(ctx, id)
	if err != nil {
		return ProductOption{}, err
	}
	var existing []ProductOption
	if source.TemplateID != uuid.Nil {
		byTemplate, err := r.ListProductOptionsByTemplateIDs(ctx, []uuid.UUID{source.TemplateID})
		if err != nil {
			return ProductOption{}, err
		}
		existing = byTemplate[source.TemplateID]
	} else if existing, err = r.ListProductOptionsByProductID(ctx, source.ProductID); err != nil {
		return ProductOption{}, err
	}
	nextSort := int32(0)
	for _, o := range existing {
		if o.SortOrder >= nextSort {
			nextSort = o.SortOrder + 1
		}
	}
	created, _, err := cloneOption(ctx, r, id, source.Owner(), nextSort, def, nil)
	return created, err
}

// cloneOption copies an option with its content, values and store amounts onto owner at sortOrder - the map says which
// new value each old one became, and listed, when given, keeps only the store amounts for those channels
func cloneOption(ctx context.Context, r OptionRepository, id uuid.UUID, owner OptionOwner, sortOrder int32, def string, listed map[uuid.UUID]bool) (ProductOption, map[uuid.UUID]uuid.UUID, error) {
	source, err := r.GetProductOptionByID(ctx, id)
	if err != nil {
		return ProductOption{}, nil, err
	}
	sourceContent, err := r.ListProductOptionContent(ctx, source.ID)
	if err != nil {
		return ProductOption{}, nil, err
	}
	initial, ok := pickLocalized(sourceContent, optionContentLocale, def, def)
	if !ok {
		return ProductOption{}, nil, fmt.Errorf("%w: %s", ErrOptionHasNoContent, source.ID)
	}
	keep := func(channelID uuid.UUID) bool { return listed == nil || listed[channelID] }
	valueMap := map[uuid.UUID]uuid.UUID{}

	created, err := r.CreateProductOption(ctx, AddProductOptionInput{
		ProductID:      owner.ProductID,
		TemplateID:     owner.TemplateID,
		InputType:      source.InputType,
		IsRequired:     source.IsRequired,
		SortOrder:      sortOrder,
		PriceValue:     source.PriceValue,
		PriceType:      source.PriceType,
		MaxChars:       source.MaxChars,
		MaxFileBytes:   source.MaxFileBytes,
		FileExtensions: source.FileExtensions,
		Locale:         initial.Locale,
		Title:          initial.Title,
		HelpText:       initial.HelpText,
		VisibleIf:      source.VisibleIf,
		ImageAssetID:   source.ImageAssetID,
	})
	if err != nil {
		return ProductOption{}, nil, err
	}
	for _, c := range sourceContent {
		if c.Locale == initial.Locale {
			continue
		}
		if _, err := r.UpsertProductOptionContent(ctx, UpsertProductOptionContentInput{
			OptionID: created.ID, Locale: c.Locale, Title: c.Title, HelpText: c.HelpText,
		}); err != nil {
			return ProductOption{}, nil, fmt.Errorf("clone option content %q: %w", c.Locale, err)
		}
	}
	optPrices, err := r.ListProductOptionChannelPricesByOptionIDs(ctx, []uuid.UUID{source.ID})
	if err != nil {
		return ProductOption{}, nil, err
	}
	for _, p := range optPrices[source.ID] {
		if !keep(p.ChannelID) {
			continue
		}
		if err := r.SetProductOptionChannelPrice(ctx, created.ID, p.ChannelID, &p.PriceValue); err != nil {
			return ProductOption{}, nil, err
		}
	}

	if source.InputType.IsSelect() {
		sourceValues, err := r.ListProductOptionValuesByOptionID(ctx, source.ID)
		if err != nil {
			return ProductOption{}, nil, err
		}
		valueIDs := make([]uuid.UUID, len(sourceValues))
		for i, v := range sourceValues {
			valueIDs[i] = v.ID
		}
		vcByValue, err := r.ListProductOptionValueContentByValueIDs(ctx, valueIDs)
		if err != nil {
			return ProductOption{}, nil, err
		}
		valPrices, err := r.ListProductOptionValueChannelPricesByValueIDs(ctx, valueIDs)
		if err != nil {
			return ProductOption{}, nil, err
		}
		for _, v := range sourceValues {
			vc := vcByValue[v.ID]
			vInitial, ok := pickLocalized(vc, valueContentLocale, def, def)
			if !ok {
				continue
			}
			newValue, err := r.CreateProductOptionValue(ctx, AddProductOptionValueInput{
				OptionID:        created.ID,
				SortOrder:       v.SortOrder,
				PriceValue:      v.PriceValue,
				PriceType:       v.PriceType,
				IsDefault:       v.IsDefault,
				Quantity:        v.Quantity,
				ForcedLineQty:   v.ForcedLineQty,
				SourceProductID: v.SourceProductID,
				ImageAssetID:    v.ImageAssetID,
				Locale:          vInitial.Locale,
				Title:           vInitial.Title,
			})
			if err != nil {
				return ProductOption{}, nil, fmt.Errorf("clone option value: %w", err)
			}
			valueMap[v.ID] = newValue.ID
			for _, p := range valPrices[v.ID] {
				if !keep(p.ChannelID) {
					continue
				}
				if err := r.SetProductOptionValueChannelPrice(ctx, newValue.ID, p.ChannelID, &p.PriceValue); err != nil {
					return ProductOption{}, nil, err
				}
			}
			for _, c := range vc {
				if c.Locale == vInitial.Locale {
					continue
				}
				if _, err := r.UpsertProductOptionValueContent(ctx, UpsertProductOptionValueContentInput{
					ValueID: newValue.ID, Locale: c.Locale, Title: c.Title,
				}); err != nil {
					return ProductOption{}, nil, fmt.Errorf("clone option value content %q: %w", c.Locale, err)
				}
			}
		}
	}
	out, err := r.GetProductOptionByID(ctx, created.ID)
	return out, valueMap, err
}

func (s *optionService) UpsertProductOptionContent(ctx context.Context, input UpsertProductOptionContentInput) (ProductOptionContent, error) {
	if strings.TrimSpace(input.Locale) == "" {
		return ProductOptionContent{}, ErrLocaleRequired
	}
	if strings.TrimSpace(input.Title) == "" {
		return ProductOptionContent{}, ErrTitleRequired
	}
	if err := s.ensureLocale(ctx, input.Locale); err != nil {
		return ProductOptionContent{}, err
	}
	opt, err := s.repo.GetProductOptionByID(ctx, input.OptionID)
	if err != nil {
		return ProductOptionContent{}, err
	}
	content, err := s.repo.UpsertProductOptionContent(ctx, input)
	return content, s.announced(ctx, opt.Owner(), err)
}

// DeleteProductOptionContent: removing the default-locale translation while other translations still exist is rejected - same rule as ProductContent / BundleOptionContent
func (s *optionService) DeleteProductOptionContent(ctx context.Context, optionID uuid.UUID, locale string) error {
	if err := s.refuseDefaultLocaleDelete(ctx, locale, func() ([]string, error) {
		rows, err := s.repo.ListProductOptionContent(ctx, optionID)
		return localesOf(rows, optionContentLocale), err
	}); err != nil {
		return err
	}
	opt, err := s.repo.GetProductOptionByID(ctx, optionID)
	if err != nil {
		return err
	}
	return s.announced(ctx, opt.Owner(), s.repo.DeleteProductOptionContent(ctx, optionID, locale))
}

// ==================== Option values ====================

// AddProductOptionValue adds one pre-defined choice to a select-family option (dropdown/radio/checkbox/multiselect) - Single-select options reject a second default; multi-select options allow zero or more - When SourceProductID is set, the referenced product must exist and be of kind SIMPLE, and PriceType must be FIXED - see validateValueSourceProduct
func (s *optionService) AddProductOptionValue(ctx context.Context, input AddProductOptionValueInput) (ProductOptionValue, error) {
	if strings.TrimSpace(input.Title) == "" {
		return ProductOptionValue{}, ErrTitleRequired
	}
	if err := validUnitQuantity(input.Quantity); err != nil {
		return ProductOptionValue{}, err
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
		return ProductOptionValue{}, ErrLocaleRequired
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && input.Locale != def {
		return ProductOptionValue{}, fmt.Errorf("%w: got %q, want %q", ErrLocaleMustBeDefault, input.Locale, def)
	}
	if err := s.validateValueSourceProduct(ctx, input.SourceProductID, input.PriceType); err != nil {
		return ProductOptionValue{}, err
	}
	if err := s.checkOptionImage(ctx, input.ImageAssetID); err != nil {
		return ProductOptionValue{}, err
	}
	var created ProductOptionValue
	err = s.inTx(ctx, func(r Repository) error {
		if input.IsDefault {
			if err := addingOptionDefault(ctx, r, option.ID); err != nil {
				return err
			}
		}
		created, err = r.CreateProductOptionValue(ctx, input)
		return err
	})
	return created, s.announced(ctx, option.Owner(), err)
}

// A single-select option takes one default - the option row lock serializes concurrent default flips
func addingOptionDefault(ctx context.Context, r OptionRepository, optionID uuid.UUID) error {
	option, err := r.GetProductOptionByIDForUpdate(ctx, optionID)
	if err != nil {
		return err
	}
	if !option.InputType.IsSingleSelect() {
		return nil
	}
	n, err := r.CountProductOptionValueDefaultsForOption(ctx, optionID)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: option already has a default value", ErrOptionMultiDefaultOnSingle)
	}
	return nil
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
		if err := validUnitQuantity(*input.Quantity); err != nil {
			return ProductOptionValue{}, err
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
	finalImage, err := s.nextOptionImage(ctx, current.ImageAssetID, input.ImageAssetID)
	if err != nil {
		return ProductOptionValue{}, err
	}

	var updated ProductOptionValue
	err = s.inTx(ctx, func(r Repository) error {
		if finalIsDefault && !current.IsDefault {
			if err := addingOptionDefault(ctx, r, current.OptionID); err != nil {
				return err
			}
		}
		updated, err = r.UpdateProductOptionValue(ctx, current.ID, finalSortOrder, finalPriceValue, finalPriceType, finalIsDefault, finalSourceProductID, finalQuantity, finalForcedLineQty, finalImage)
		return err
	})
	return updated, s.announcedForOption(ctx, current.OptionID, err)
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
	siblings, err := s.ownerOptions(ctx, option.Owner())
	if err != nil {
		return err
	}
	for _, sib := range siblings {
		if sib.ID != option.ID && sib.TemplateID == option.TemplateID && groupReferencesValue(sib.VisibleIf, id) {
			return fmt.Errorf("%w: the visibility rule on option %q references this value - remove it first", ErrOptionVisibilityInvalid, sib.Title)
		}
	}
	if rule, err := s.ruleUsing(ctx, option.Owner(), nil, &id); err != nil || rule != nil {
		if err != nil {
			return err
		}
		return ruleInUseError(rule)
	}
	if option.TemplateID != uuid.Nil {
		users, err := s.repo.ListAllProductIDsUsingOptionTemplate(ctx, option.TemplateID)
		if err != nil {
			return err
		}
		if err := s.refuseProductLeaning(ctx, users, ProductOption{Values: []ProductOptionValue{current}}); err != nil {
			return err
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
	return s.announced(ctx, option.Owner(), s.repo.DeleteProductOptionValue(ctx, id))
}

func (s *optionService) UpsertProductOptionValueContent(ctx context.Context, input UpsertProductOptionValueContentInput) (ProductOptionValueContent, error) {
	if strings.TrimSpace(input.Locale) == "" {
		return ProductOptionValueContent{}, ErrLocaleRequired
	}
	if strings.TrimSpace(input.Title) == "" {
		return ProductOptionValueContent{}, ErrTitleRequired
	}
	if err := s.ensureLocale(ctx, input.Locale); err != nil {
		return ProductOptionValueContent{}, err
	}
	val, err := s.repo.GetProductOptionValueByID(ctx, input.ValueID)
	if err != nil {
		return ProductOptionValueContent{}, err
	}
	content, err := s.repo.UpsertProductOptionValueContent(ctx, input)
	return content, s.announcedForOption(ctx, val.OptionID, err)
}

func (s *optionService) DeleteProductOptionValueContent(ctx context.Context, valueID uuid.UUID, locale string) error {
	if err := s.refuseDefaultLocaleDelete(ctx, locale, func() ([]string, error) {
		rows, err := s.repo.ListProductOptionValueContent(ctx, valueID)
		return localesOf(rows, valueContentLocale), err
	}); err != nil {
		return err
	}
	val, err := s.repo.GetProductOptionValueByID(ctx, valueID)
	if err != nil {
		return err
	}
	return s.announcedForOption(ctx, val.OptionID, s.repo.DeleteProductOptionValueContent(ctx, valueID, locale))
}

// ==================== Read-side: full options view ====================

// GetProductOptions loads every option for a product, hydrating each with its values (for select-family) and resolved titles (request locale → default-locale fallback) - Mirrors GetBundleOptions
func (s *optionService) GetProductOptions(ctx context.Context, productID uuid.UUID, locale string) ([]ProductOption, error) {
	byProduct, err := s.GetProductOptionsByProductIDs(ctx, []uuid.UUID{productID}, locale)
	if err != nil {
		return nil, err
	}
	if opts := byProduct[productID]; opts != nil {
		return opts, nil
	}
	return []ProductOption{}, nil
}

// A product with no options is absent from the map - a product's list leads with the options of the templates it uses,
// its type's and then its own picks, in their order, then its own options
func (s *optionService) GetProductOptionsByProductIDs(ctx context.Context, productIDs []uuid.UUID, locale string) (map[uuid.UUID][]ProductOption, error) {
	productIDs = distinctIDs(productIDs)
	own, err := s.repo.ListProductOptionsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	uses, err := s.repo.ListEffectiveOptionTemplates(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	assigned := make(map[uuid.UUID][]uuid.UUID, len(uses))
	var templateIDs []uuid.UUID
	for pid, list := range uses {
		assigned[pid] = templateIDsOf(list)
		templateIDs = append(templateIDs, assigned[pid]...)
	}
	templateIDs = distinctIDs(templateIDs)
	byTemplate := map[uuid.UUID][]ProductOption{}
	templates := map[uuid.UUID]OptionTemplate{}
	if len(templateIDs) > 0 {
		if byTemplate, err = s.repo.ListProductOptionsByTemplateIDs(ctx, templateIDs); err != nil {
			return nil, err
		}
		if templates, err = s.repo.ListOptionTemplatesByIDs(ctx, templateIDs); err != nil {
			return nil, err
		}
	}

	var rows []ProductOption
	for _, pid := range productIDs {
		rows = append(rows, own[pid]...)
	}
	for _, tid := range templateIDs {
		rows = append(rows, byTemplate[tid]...)
	}
	if len(rows) == 0 {
		return map[uuid.UUID][]ProductOption{}, nil
	}
	if err := s.hydrateOptions(ctx, rows, locale, templates); err != nil {
		return nil, err
	}
	hydrated := make(map[uuid.UUID]ProductOption, len(rows))
	for _, o := range rows {
		hydrated[o.ID] = o
	}

	out := make(map[uuid.UUID][]ProductOption, len(productIDs))
	for _, pid := range productIDs {
		var list []ProductOption
		for _, tid := range assigned[pid] {
			for _, o := range byTemplate[tid] {
				h := hydrated[o.ID]
				h.TemplateName = templates[tid].Name
				h.SortOrder = int32(templateSortBand + len(list))
				list = append(list, h)
			}
		}
		for _, o := range own[pid] {
			list = append(list, hydrated[o.ID])
		}
		if len(list) > 0 {
			out[pid] = list
		}
	}
	return out, nil
}

// hydrateOptions fills in content, values, store amounts and currency on bare option rows, in place - a product's
// option is in the product's currency, a template's in the template's
func (s *optionService) hydrateOptions(ctx context.Context, options []ProductOption, locale string, templates map[uuid.UUID]OptionTemplate) error {
	if len(options) == 0 {
		return nil
	}
	defLocale := s.resolveDefaultLocale(ctx)
	if strings.TrimSpace(locale) == "" {
		locale = defLocale
	}
	optionIDs := make([]uuid.UUID, len(options))
	var ownerIDs []uuid.UUID
	for i, o := range options {
		optionIDs[i] = o.ID
		if o.ProductID != uuid.Nil {
			ownerIDs = append(ownerIDs, o.ProductID)
		}
	}

	contentByOption, err := s.repo.ListProductOptionContentByOptionIDs(ctx, optionIDs)
	if err != nil {
		return err
	}
	valuesByOption, err := s.repo.ListProductOptionValuesByOptionIDs(ctx, optionIDs)
	if err != nil {
		return err
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
			return err
		}
	}

	// A since-deleted source product simply drops out of the maps and the value renders unlinked
	sourceByID, sourceContentByID, err := s.loadProductsWithContent(ctx, sourceProductIDs, locale, false)
	if err != nil {
		return fmt.Errorf("option value sources: %w", err)
	}
	owners, err := s.repo.ListProductRowsByIDs(ctx, distinctIDs(ownerIDs))
	if err != nil {
		return fmt.Errorf("option owners: %w", err)
	}
	optPrices, err := s.repo.ListProductOptionChannelPricesByOptionIDs(ctx, optionIDs)
	if err != nil {
		return err
	}
	valPrices, err := s.repo.ListProductOptionValueChannelPricesByValueIDs(ctx, valueIDs)
	if err != nil {
		return err
	}

	for i := range options {
		opt := &options[i]
		opt.AllContent = contentByOption[opt.ID]
		oc, _ := pickLocalized(opt.AllContent, optionContentLocale, locale, defLocale)
		opt.Title, opt.HelpText = oc.Title, oc.HelpText
		if opt.TemplateID != uuid.Nil {
			opt.Currency = templates[opt.TemplateID].CurrencyCode
			opt.TemplateName = templates[opt.TemplateID].Name
		} else {
			opt.Currency = owners[opt.ProductID].DefaultCurrencyCode
		}
		opt.ChannelPrices = optPrices[opt.ID]

		vals := valuesByOption[opt.ID]
		for j := range vals {
			v := &vals[j]
			v.AllContent = valueContentByValue[v.ID]
			vc, _ := pickLocalized(v.AllContent, valueContentLocale, locale, defLocale)
			v.Title = vc.Title
			v.ChannelPrices = valPrices[v.ID]
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
	return nil
}

func distinctIDs(ids []uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func optionContentLocale(c ProductOptionContent) string     { return c.Locale }
func valueContentLocale(c ProductOptionValueContent) string { return c.Locale }

// A picture has to be a finished media-library upload - a pending one has no file yet
func (s *optionService) checkOptionImage(ctx context.Context, id *uuid.UUID) error {
	if id == nil || *id == uuid.Nil {
		return nil
	}
	ok, err := s.repo.MediaAssetReady(ctx, *id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrOptionImageInvalid
	}
	return nil
}

// The picture after an update - nil leaves it, a zero id clears it, anything else replaces it
func (s *optionService) nextOptionImage(ctx context.Context, current, next *uuid.UUID) (*uuid.UUID, error) {
	switch {
	case next == nil:
		return current, nil
	case *next == uuid.Nil:
		return nil, nil
	}
	if err := s.checkOptionImage(ctx, next); err != nil {
		return nil, err
	}
	id := *next
	return &id, nil
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
		return ErrOptionPriceIncomplete
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

func validatePercentRange(t ProductOptionPriceType, v int32) error {
	if t != OptionPricePercent {
		return nil
	}
	if v < MinOptionPercent || v > MaxOptionPercent {
		return fmt.Errorf("%w: percent adjustment must be between %d and %d, got %d", ErrInvalidProductOptionPriceType, MinOptionPercent, MaxOptionPercent, v)
	}
	return nil
}

// Forced line quantities multiply into order lines - fractional forces would make "Set of 4" ship 2.5 units
func validateForcedLineQty(q *float64) error {
	if q == nil {
		return nil
	}
	if math.IsNaN(*q) || *q <= 0 || *q > MaxStructureQuantity || *q != math.Trunc(*q) {
		return fmt.Errorf("%w: got %v", ErrInvalidForcedLineQty, *q)
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

// The options a condition or rule on this owner may read, values attached - a product's include its templates', which
// always come first, so its own options may read them whatever their order
func (s *optionService) ownerOptions(ctx context.Context, owner OptionOwner) ([]ProductOption, error) {
	if owner.IsTemplate() {
		return s.GetOptionTemplateOptions(ctx, owner.TemplateID, "")
	}
	return s.GetProductOptions(ctx, owner.ProductID, "")
}

// validateOptionVisibility structurally checks a visibility condition: known ops/logic, no self-reference, parents exist on the product with STRICTLY lower sort_order (canonical order (sort_order, created_at) - the strict-order rule is what makes evaluation a cycle-free single pass), anyOf/noneOf only on select-family parents with value_ids that belong to them
func validateOptionVisibility(g *OptionConditionGroup, ownOrder int32, ownID uuid.UUID, siblings []ProductOption) error {
	byID := make(map[uuid.UUID]ProductOption, len(siblings))
	for _, sib := range siblings {
		byID[sib.ID] = sib
	}
	return validateVisibilityGroup(g, ownOrder, ownID, byID)
}

func validateVisibilityGroup(g *OptionConditionGroup, ownOrder int32, ownID uuid.UUID, byID map[uuid.UUID]ProductOption) error {
	if g == nil {
		return nil
	}
	if g.Logic != "" && g.Logic != OptionLogicAnd && g.Logic != OptionLogicOr {
		return fmt.Errorf("%w: logic must be \"and\" or \"or\"", ErrOptionVisibilityInvalid)
	}
	if len(g.Rules) == 0 && len(g.Groups) == 0 {
		return fmt.Errorf("%w: a group needs at least one rule", ErrOptionVisibilityInvalid)
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
		if r.Op == OptionRuleAnyOf || r.Op == OptionRuleNoneOf {
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
		if err := validateVisibilityGroup(&g.Groups[i], ownOrder, ownID, byID); err != nil {
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

func (s *optionService) SetProductOptionChannelPrice(ctx context.Context, optionID, channelID uuid.UUID, priceValue *int32) (ProductOption, error) {
	opt, err := s.repo.GetProductOptionByID(ctx, optionID)
	if err != nil {
		return ProductOption{}, err
	}
	if priceValue != nil && (opt.PriceType == nil || *opt.PriceType != OptionPriceFixed) {
		return ProductOption{}, ErrOptionChannelPriceNotFixed
	}
	if err := s.checkChannelSurcharge(ctx, opt.Owner(), channelID, priceValue); err != nil {
		return ProductOption{}, err
	}
	if err := s.repo.SetProductOptionChannelPrice(ctx, optionID, channelID, priceValue); err != nil {
		return ProductOption{}, err
	}
	return opt, s.announced(ctx, opt.Owner(), nil)
}

// Sets or (nil) clears a value's FIXED surcharge, or a linked value's markup, for one channel the product is listed on
func (s *optionService) SetProductOptionValueChannelPrice(ctx context.Context, valueID, channelID uuid.UUID, priceValue *int32) (ProductOptionValue, error) {
	val, err := s.repo.GetProductOptionValueByID(ctx, valueID)
	if err != nil {
		return ProductOptionValue{}, err
	}
	if priceValue != nil && val.PriceType == OptionPricePercent && val.SourceProductID == nil {
		return ProductOptionValue{}, ErrOptionChannelPriceNotFixed
	}
	opt, err := s.repo.GetProductOptionByID(ctx, val.OptionID)
	if err != nil {
		return ProductOptionValue{}, err
	}
	if err := s.checkChannelSurcharge(ctx, opt.Owner(), channelID, priceValue); err != nil {
		return ProductOptionValue{}, err
	}
	if err := s.repo.SetProductOptionValueChannelPrice(ctx, valueID, channelID, priceValue); err != nil {
		return ProductOptionValue{}, err
	}
	return val, s.announced(ctx, opt.Owner(), nil)
}

func (s *optionService) checkChannelSurcharge(ctx context.Context, owner OptionOwner, channelID uuid.UUID, priceValue *int32) error {
	if priceValue != nil && (*priceValue > MaxOptionChannelSurcharge || *priceValue < -MaxOptionChannelSurcharge) {
		return fmt.Errorf("%w: %d", ErrOptionChannelPriceRange, *priceValue)
	}
	if priceValue == nil {
		return nil // clearing is always allowed, even for a channel the product has since left
	}
	if owner.IsTemplate() {
		if known, err := s.knownChannels(ctx, []uuid.UUID{channelID}); err != nil || !known[channelID] {
			return errors.Join(ErrOptionChannelNotListed, err)
		}
		return nil
	}
	listings, err := s.core.repo.ListProductChannelListingsByProduct(ctx, owner.ProductID)
	if err != nil {
		return err
	}
	for _, l := range listings {
		if l.ChannelID == channelID {
			return nil
		}
	}
	return ErrOptionChannelNotListed
}

// Every option write changes what the product page renders and what the cart charges, so a successful one announces the product - or every product a template reaches - err passes through untouched
func (s *optionService) announced(ctx context.Context, owner OptionOwner, err error) error {
	if err != nil {
		return err
	}
	if !owner.IsTemplate() {
		s.publishProductUpdatedByID(ctx, owner.ProductID, ProductFieldOptions)
		return nil
	}
	ids, lerr := s.repo.ListAllProductIDsUsingOptionTemplate(ctx, owner.TemplateID)
	if lerr != nil {
		applog.From(ctx).Warn("list products using option template", "template_id", owner.TemplateID, "err", lerr) // the write stands - only the cache purge is lost
		return nil
	}
	s.publishProductsUpdated(ctx, ids, ProductFieldOptions)
	return nil
}

func (s *optionService) announcedForOption(ctx context.Context, optionID uuid.UUID, err error) error {
	if err != nil {
		return err
	}
	opt, oerr := s.repo.GetProductOptionByID(ctx, optionID)
	if oerr != nil {
		applog.From(ctx).Warn("announce option change", "option_id", optionID, "err", oerr) // the write stands - only the cache purge is lost
		return nil
	}
	return s.announced(ctx, opt.Owner(), nil)
}
