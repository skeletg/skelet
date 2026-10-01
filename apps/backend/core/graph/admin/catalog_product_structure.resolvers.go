package admin

import (
	"context"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

func (r *mutationResolver) AddBundleOption(ctx context.Context, input model.AddBundleOptionInput) (*model.Product, error) {
	productID, err := parseID(input.ProductID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, productID); err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.AddBundleOption(ctx, catalog.AddBundleOptionInput{
		ProductID:     productID,
		InputType:     catalog.BundleInputType(strings.ToLower(string(input.InputType))),
		IsRequired:    boolOrDefault(input.IsRequired, true),
		SortOrder:     int32Or(input.SortOrder, 0),
		MinSelections: int32Or(input.MinSelections, 0),
		MaxSelections: input.MaxSelections,
		Locale:        input.Locale,
		Title:         input.Title,
	})
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, opt.ProductID)
}

func (r *mutationResolver) UpdateBundleOption(ctx context.Context, input model.UpdateBundleOptionInput) (*model.Product, error) {
	id, err := parseID(input.ID, "option")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForBundleOption(ctx, id); err != nil {
		return nil, err
	}
	domainInput := catalog.UpdateBundleOptionInput{
		ID:                 id,
		IsRequired:         input.IsRequired,
		SortOrder:          input.SortOrder,
		MinSelections:      input.MinSelections,
		MaxSelections:      input.MaxSelections,
		ClearMaxSelections: input.ClearMaxSelections != nil && *input.ClearMaxSelections,
	}
	if input.InputType != nil {
		t := catalog.BundleInputType(strings.ToLower(string(*input.InputType)))
		domainInput.InputType = &t
	}
	opt, err := r.CatalogSvc.UpdateBundleOption(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, opt.ProductID)
}

func (r *mutationResolver) UpsertBundleOptionContent(ctx context.Context, optionID string, locale string, title string) (*model.Product, error) {
	id, err := parseID(optionID, "option")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForBundleOption(ctx, id); err != nil {
		return nil, err
	}
	if _, err := r.CatalogSvc.UpsertBundleOptionContent(ctx, catalog.UpsertBundleOptionContentInput{OptionID: id, Locale: locale, Title: title}); err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetBundleOption(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, opt.ProductID)
}

func (r *mutationResolver) DeleteBundleOptionContent(ctx context.Context, optionID string, locale string) (*model.Product, error) {
	id, err := parseID(optionID, "option")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForBundleOption(ctx, id); err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetBundleOption(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.DeleteBundleOptionContent(ctx, id, locale); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, opt.ProductID)
}

func (r *mutationResolver) DeleteBundleOption(ctx context.Context, id string) (*model.Product, error) {
	optID, err := parseID(id, "option")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForBundleOption(ctx, optID); err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetBundleOption(ctx, optID)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.DeleteBundleOption(ctx, optID); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, opt.ProductID)
}

func (r *mutationResolver) AddBundleSelection(ctx context.Context, input model.AddBundleSelectionInput) (*model.Product, error) {
	optionID, err := parseID(input.OptionID, "option")
	if err != nil {
		return nil, err
	}
	productID, err := parseID(input.ProductID, "product")
	if err != nil {
		return nil, err
	}
	// Gated on the bundle that owns the option - productID below is the product being selected INTO it
	if err := r.requireCatalogManageForBundleOption(ctx, optionID); err != nil {
		return nil, err
	}
	sel, err := r.CatalogSvc.AddBundleSelection(ctx, catalog.AddBundleSelectionInput{
		OptionID:  optionID,
		ProductID: productID,
		Quantity:  float64Or(input.Quantity, catalog.DefaultStructureQuantity),
		IsDefault: boolOrFalse(input.IsDefault),
		SortOrder: int32Or(input.SortOrder, 0),
	})
	if err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetBundleOption(ctx, sel.OptionID)
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, opt.ProductID)
}

func (r *mutationResolver) UpdateBundleSelection(ctx context.Context, input model.UpdateBundleSelectionInput) (*model.Product, error) {
	id, err := parseID(input.ID, "selection")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForBundleSelection(ctx, id); err != nil {
		return nil, err
	}
	sel, err := r.CatalogSvc.UpdateBundleSelection(ctx, catalog.UpdateBundleSelectionInput{
		ID:        id,
		Quantity:  input.Quantity,
		IsDefault: input.IsDefault,
		SortOrder: input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetBundleOption(ctx, sel.OptionID)
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, opt.ProductID)
}

func (r *mutationResolver) DeleteBundleSelection(ctx context.Context, id string) (*model.Product, error) {
	selID, err := parseID(id, "selection")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForBundleSelection(ctx, selID); err != nil {
		return nil, err
	}
	sel, err := r.CatalogSvc.GetBundleSelection(ctx, selID)
	if err != nil {
		return nil, err
	}
	opt, err := r.CatalogSvc.GetBundleOption(ctx, sel.OptionID)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.DeleteBundleSelection(ctx, selID); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, opt.ProductID)
}

func (r *mutationResolver) AddGroupedMember(ctx context.Context, input model.AddGroupedMemberInput) (*model.Product, error) {
	parentID, err := parseID(input.ParentID, "parent")
	if err != nil {
		return nil, err
	}
	memberID, err := parseID(input.MemberID, "member")
	if err != nil {
		return nil, err
	}
	// The parent is what gets mutated - memberID is only referenced
	if err := r.requireCatalogManageOnAllListingChannels(ctx, parentID); err != nil {
		return nil, err
	}
	member, err := r.CatalogSvc.AddGroupedMember(ctx, catalog.AddGroupedMemberInput{
		ParentID:        parentID,
		MemberID:        memberID,
		DefaultQuantity: float64Or(input.DefaultQuantity, catalog.DefaultStructureQuantity),
		SortOrder:       int32Or(input.SortOrder, 0),
	})
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, member.ParentID)
}

func (r *mutationResolver) UpdateGroupedMember(ctx context.Context, input model.UpdateGroupedMemberInput) (*model.Product, error) {
	id, err := parseID(input.ID, "member")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForGroupedMember(ctx, id); err != nil {
		return nil, err
	}
	member, err := r.CatalogSvc.UpdateGroupedMember(ctx, catalog.UpdateGroupedMemberInput{
		ID:              id,
		DefaultQuantity: input.DefaultQuantity,
		SortOrder:       input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, member.ParentID)
}

func (r *mutationResolver) RemoveGroupedMember(ctx context.Context, id string) (*model.Product, error) {
	memberID, err := parseID(id, "member")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForGroupedMember(ctx, memberID); err != nil {
		return nil, err
	}
	member, err := r.CatalogSvc.GetGroupedMember(ctx, memberID)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.RemoveGroupedMember(ctx, memberID); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, member.ParentID)
}

// AddKitComponent wires a SIMPLE product into a kit's fixed component list.
func (r *mutationResolver) AddKitComponent(ctx context.Context, input model.AddKitComponentInput) (*model.Product, error) {
	productID, err := parseID(input.ProductID, "product")
	if err != nil {
		return nil, err
	}
	componentID, err := parseID(input.ComponentProductID, "component product")
	if err != nil {
		return nil, err
	}
	// The kit is what gets mutated - componentID is only referenced
	if err := r.requireCatalogManageOnAllListingChannels(ctx, productID); err != nil {
		return nil, err
	}
	comp, err := r.CatalogSvc.AddKitComponent(ctx, catalog.AddKitComponentInput{
		ProductID:          productID,
		ComponentProductID: componentID,
		Quantity:           float64Or(input.Quantity, catalog.DefaultStructureQuantity),
		SortOrder:          int32Or(input.SortOrder, 0),
	})
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, comp.ProductID)
}

// UpdateKitComponent applies pointer-style updates to a kit component.
func (r *mutationResolver) UpdateKitComponent(ctx context.Context, input model.UpdateKitComponentInput) (*model.Product, error) {
	id, err := parseID(input.ID, "kit component")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForKitComponent(ctx, id); err != nil {
		return nil, err
	}
	comp, err := r.CatalogSvc.UpdateKitComponent(ctx, catalog.UpdateKitComponentInput{
		ID:        id,
		Quantity:  input.Quantity,
		SortOrder: input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, comp.ProductID)
}

// RemoveKitComponent removes one component from a kit.
func (r *mutationResolver) RemoveKitComponent(ctx context.Context, id string) (*model.Product, error) {
	compID, err := parseID(id, "kit component")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageForKitComponent(ctx, compID); err != nil {
		return nil, err
	}
	comp, err := r.CatalogSvc.GetKitComponent(ctx, compID)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.DeleteKitComponent(ctx, compID); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, comp.ProductID)
}
