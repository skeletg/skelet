package admin

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) AddProductOption(ctx context.Context, input model.AddProductOptionInput) (model.ProductOptionOwner, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	owner, err := r.requireOptionOwnerManage(ctx, input.ProductID, input.TemplateID)
	if err != nil {
		return nil, err
	}
	domain := catalog.AddProductOptionInput{
		ProductID:  owner.ProductID,
		TemplateID: owner.TemplateID,
		InputType:  catalog.ProductOptionInputType(strings.ToLower(string(input.InputType))),
		IsRequired: boolOrFalse(input.IsRequired),
		SortOrder:  int32Or(input.SortOrder, 0),
		Locale:     input.Locale,
		Title:      input.Title,
		HelpText:   ptr.Deref(input.HelpText),
	}
	if input.PriceValue != nil {
		v := *input.PriceValue
		domain.PriceValue = &v
	}
	if input.PriceType != nil {
		t := catalog.ProductOptionPriceType(strings.ToLower(string(*input.PriceType)))
		domain.PriceType = &t
	}
	if input.MaxChars != nil {
		v := *input.MaxChars
		domain.MaxChars = &v
	}
	if input.MaxFileBytes != nil {
		v := int64(*input.MaxFileBytes)
		domain.MaxFileBytes = &v
	}
	if input.FileExtensions != nil {
		domain.FileExtensions = *input.FileExtensions
	}
	if domain.VisibleIf, err = visibleIfFromInput(input.VisibleIf); err != nil {
		return nil, err
	}
	if domain.ImageAssetID, err = optionImageInput(input.ImageAssetID, false); err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.AddProductOption(ctx, domain)
	if err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) UpdateProductOption(ctx context.Context, input model.UpdateProductOptionInput) (model.ProductOptionOwner, error) {
	id, err := parseID(input.ID, "option")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForProductOption(ctx, id); err != nil {
		return nil, err
	}
	domain := catalog.UpdateProductOptionInput{
		ID:             id,
		IsRequired:     input.IsRequired,
		SortOrder:      input.SortOrder,
		PriceValue:     input.PriceValue,
		MaxChars:       input.MaxChars,
		FileExtensions: input.FileExtensions,
	}
	if input.InputType != nil {
		t := catalog.ProductOptionInputType(strings.ToLower(string(*input.InputType)))
		domain.InputType = &t
	}
	if input.PriceType != nil {
		t := catalog.ProductOptionPriceType(strings.ToLower(string(*input.PriceType)))
		domain.PriceType = &t
	}
	if input.MaxFileBytes != nil {
		v := int64(*input.MaxFileBytes)
		domain.MaxFileBytes = &v
	}
	if domain.VisibleIf, err = visibleIfFromInput(input.VisibleIf); err != nil {
		return nil, err
	}
	if domain.ImageAssetID, err = optionImageInput(input.ImageAssetID, true); err != nil {
		return nil, err
	}
	if input.ClearVisibleIf != nil && *input.ClearVisibleIf {
		domain.ClearVisibleIf = true
	}
	if input.ClearPrice != nil && *input.ClearPrice {
		domain.ClearPrice = true
	}
	if input.ClearMaxChars != nil && *input.ClearMaxChars {
		domain.ClearMaxChars = true
	}
	if input.ClearMaxFileBytes != nil && *input.ClearMaxFileBytes {
		domain.ClearMaxFileBytes = true
	}
	if input.ClearFileExtensions != nil && *input.ClearFileExtensions {
		domain.ClearFileExtensions = true
	}
	opt, err := r.CatalogSvc.UpdateProductOption(ctx, domain)
	if err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) UpsertProductOptionContent(ctx context.Context, optionID string, locale string, title string, helpText *string) (model.ProductOptionOwner, error) {
	id, err := parseID(optionID, "option")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForProductOption(ctx, id); err != nil {
		return nil, err
	}
	if _, err := r.CatalogSvc.UpsertProductOptionContent(ctx, catalog.UpsertProductOptionContentInput{
		OptionID: id,
		Locale:   locale,
		Title:    title,
		HelpText: ptr.Deref(helpText),
	}); err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetProductOption(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) DeleteProductOptionContent(ctx context.Context, optionID string, locale string) (model.ProductOptionOwner, error) {
	id, err := parseID(optionID, "option")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForProductOption(ctx, id); err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetProductOption(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.DeleteProductOptionContent(ctx, id, locale); err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) DeleteProductOption(ctx context.Context, id string) (model.ProductOptionOwner, error) {
	optID, err := parseID(id, "option")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForProductOption(ctx, optID); err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetProductOption(ctx, optID)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.DeleteProductOption(ctx, optID); err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) DuplicateProductOption(ctx context.Context, id string) (model.ProductOptionOwner, error) {
	optID, err := parseID(id, "option")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForProductOption(ctx, optID); err != nil {
		return nil, err
	}
	created, err := r.CatalogSvc.DuplicateProductOption(ctx, optID)
	if err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, created.Owner())
}

func (r *mutationResolver) AddProductOptionValue(ctx context.Context, input model.AddProductOptionValueInput) (model.ProductOptionOwner, error) {
	optionID, err := parseID(input.OptionID, "option")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForProductOption(ctx, optionID); err != nil {
		return nil, err
	}
	priceType := catalog.OptionPriceFixed
	if input.PriceType != nil {
		priceType = catalog.ProductOptionPriceType(strings.ToLower(string(*input.PriceType)))
	}
	domain := catalog.AddProductOptionValueInput{
		OptionID:   optionID,
		SortOrder:  int32Or(input.SortOrder, 0),
		PriceValue: int32Or(input.PriceValue, 0),
		PriceType:  priceType,
		IsDefault:  boolOrFalse(input.IsDefault),
		Quantity:   float64Or(input.Quantity, catalog.DefaultStructureQuantity),
		Locale:     input.Locale,
		Title:      input.Title,
	}
	domain.ForcedLineQty = input.ForcedLineQty
	if input.SourceProductID != nil && *input.SourceProductID != "" {
		spid, err := parseID(*input.SourceProductID, "source product")
		if err != nil {
			return nil, err
		}
		domain.SourceProductID = &spid
	}
	if domain.ImageAssetID, err = optionImageInput(input.ImageAssetID, false); err != nil {
		return nil, err
	}
	val, err := r.CatalogSvc.AddProductOptionValue(ctx, domain)
	if err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetProductOption(ctx, val.OptionID)
	if err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) UpdateProductOptionValue(ctx context.Context, input model.UpdateProductOptionValueInput) (model.ProductOptionOwner, error) {
	id, err := parseID(input.ID, "value")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForProductOptionValue(ctx, id); err != nil {
		return nil, err
	}
	domain := catalog.UpdateProductOptionValueInput{
		ID:            id,
		SortOrder:     input.SortOrder,
		PriceValue:    input.PriceValue,
		IsDefault:     input.IsDefault,
		Quantity:      input.Quantity,
		ForcedLineQty: input.ForcedLineQty,
	}
	if input.PriceType != nil {
		t := catalog.ProductOptionPriceType(strings.ToLower(string(*input.PriceType)))
		domain.PriceType = &t
	}
	if input.SourceProductID != nil {
		// Empty string is the "unlink" sentinel (zero UUID), non-empty must parse. Nil means no change - handled by leaving
		// the pointer nil on the domain input
		if *input.SourceProductID == "" {
			zero := uuid.UUID{}
			domain.SourceProductID = &zero
		} else {
			spid, err := parseID(*input.SourceProductID, "source product")
			if err != nil {
				return nil, err
			}
			domain.SourceProductID = &spid
		}
	}
	if domain.ImageAssetID, err = optionImageInput(input.ImageAssetID, true); err != nil {
		return nil, err
	}
	val, err := r.CatalogSvc.UpdateProductOptionValue(ctx, domain)
	if err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetProductOption(ctx, val.OptionID)
	if err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) UpsertProductOptionValueContent(ctx context.Context, valueID string, locale string, title string) (model.ProductOptionOwner, error) {
	id, err := parseID(valueID, "value")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForProductOptionValue(ctx, id); err != nil {
		return nil, err
	}
	if _, err := r.CatalogSvc.UpsertProductOptionValueContent(ctx, catalog.UpsertProductOptionValueContentInput{ValueID: id, Locale: locale, Title: title}); err != nil {
		return nil, err
	}
	val, err := r.CatalogSvc.GetProductOptionValue(ctx, id)
	if err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetProductOption(ctx, val.OptionID)
	if err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) DeleteProductOptionValueContent(ctx context.Context, valueID string, locale string) (model.ProductOptionOwner, error) {
	id, err := parseID(valueID, "value")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForProductOptionValue(ctx, id); err != nil {
		return nil, err
	}
	val, err := r.CatalogSvc.GetProductOptionValue(ctx, id)
	if err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetProductOption(ctx, val.OptionID)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.DeleteProductOptionValueContent(ctx, id, locale); err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) DeleteProductOptionValue(ctx context.Context, id string) (model.ProductOptionOwner, error) {
	valID, err := parseID(id, "value")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForProductOptionValue(ctx, valID); err != nil {
		return nil, err
	}
	val, err := r.CatalogSvc.GetProductOptionValue(ctx, valID)
	if err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetProductOption(ctx, val.OptionID)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.DeleteProductOptionValue(ctx, valID); err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) SetProductOptionChannelPrice(ctx context.Context, optionID string, channelID string, priceValue *int32) (model.ProductOptionOwner, error) {
	id, err := parseID(optionID, "option")
	if err != nil {
		return nil, err
	}
	chID, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	opt, err := r.requireCatalogManageForOptionChannelPrice(ctx, id, chID)
	if err != nil {
		return nil, err
	}
	if _, err := r.CatalogSvc.SetProductOptionChannelPrice(ctx, id, chID, priceValue); err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}

func (r *mutationResolver) SetProductOptionValueChannelPrice(ctx context.Context, valueID string, channelID string, priceValue *int32) (model.ProductOptionOwner, error) {
	id, err := parseID(valueID, "value")
	if err != nil {
		return nil, err
	}
	chID, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	val, err := r.CatalogSvc.GetProductOptionValue(ctx, id)
	if err != nil {
		return nil, err
	}
	opt, err := r.requireCatalogManageForOptionChannelPrice(ctx, val.OptionID, chID)
	if err != nil {
		return nil, err
	}
	if _, err := r.CatalogSvc.SetProductOptionValueChannelPrice(ctx, id, chID, priceValue); err != nil {
		return nil, err
	}
	return r.reloadOwner(ctx, opt.Owner())
}
