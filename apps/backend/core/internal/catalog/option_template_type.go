package catalog

import (
	"context"
	"slices"

	"github.com/google/uuid"
)

// A product type's templates: handed to every product of the type that carries options, ahead of the templates the
// product picks itself. They live on the type, so nothing is copied - a product's lists read them through
// ListEffectiveOptionTemplates

// The kinds a cart resolves options for - the only ones a template reaches, picked or handed over by their type
var optionCarryingKinds = []ProductKind{KindSimple, KindConfigurable}

func optionCarryingKindNames() []string {
	out := make([]string, len(optionCarryingKinds))
	for i, k := range optionCarryingKinds {
		out[i] = string(k)
	}
	return out
}

// The templates a product gets from its product type, ahead of its own - none for a kind that carries no options
func (s *optionService) GetInheritedOptionTemplates(ctx context.Context, productID uuid.UUID) ([]OptionTemplate, error) {
	uses, err := s.repo.ListEffectiveOptionTemplates(ctx, []uuid.UUID{productID})
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, u := range uses[productID] {
		if u.FromProductType {
			ids = append(ids, u.TemplateID)
		}
	}
	return s.templatesInOrder(ctx, ids)
}

// The templates a product type hands every product of its type that carries options, in order
func (s *optionService) GetProductTypeOptionTemplates(ctx context.Context, productTypeID uuid.UUID) ([]OptionTemplate, error) {
	byType, err := s.repo.ListOptionTemplateIDsByProductTypeIDs(ctx, []uuid.UUID{productTypeID})
	if err != nil {
		return nil, err
	}
	return s.templatesInOrder(ctx, byType[productTypeID])
}

func (s *optionService) ListProductTypeIDsUsingOptionTemplate(ctx context.Context, templateID uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.ListProductTypeIDsUsingOptionTemplate(ctx, templateID)
}

// SetProductTypeOptionTemplates replaces the templates a product type hands its products, in order - one left out
// must not be something a product of the type still reads with its own options or rules, unless it picked the
// template itself too. The same list again changes nothing and tells no one
func (s *optionService) SetProductTypeOptionTemplates(ctx context.Context, productTypeID uuid.UUID, templateIDs []uuid.UUID) error {
	if err := checkTemplateList(templateIDs); err != nil {
		return err
	}
	if _, err := s.core.repo.GetProductTypeByID(ctx, productTypeID); err != nil {
		return err
	}
	found, err := s.repo.ListOptionTemplatesByIDs(ctx, templateIDs)
	if err != nil {
		return err
	}
	if len(found) != len(templateIDs) {
		return ErrOptionTemplateNotFound
	}
	byType, err := s.repo.ListOptionTemplateIDsByProductTypeIDs(ctx, []uuid.UUID{productTypeID})
	if err != nil {
		return err
	}
	current := byType[productTypeID]
	if slices.Equal(current, templateIDs) {
		return nil
	}
	products, err := s.repo.ListOptionCarryingProductIDsByType(ctx, productTypeID)
	if err != nil {
		return err
	}
	dropped := slices.DeleteFunc(slices.Clone(current), func(id uuid.UUID) bool { return slices.Contains(templateIDs, id) })
	if len(dropped) > 0 && len(products) > 0 {
		picked, err := s.repo.ListOptionTemplateAssignments(ctx, products)
		if err != nil {
			return err
		}
		for _, tid := range dropped {
			losing := slices.DeleteFunc(slices.Clone(products), func(pid uuid.UUID) bool { return slices.Contains(picked[pid], tid) })
			if err := s.refuseLeaningOnTemplate(ctx, losing, tid); err != nil {
				return err
			}
		}
	}
	if err := s.inTx(ctx, func(r Repository) error {
		return r.ReplaceProductTypeOptionTemplates(ctx, productTypeID, templateIDs)
	}); err != nil {
		return err
	}
	s.publishProductsUpdated(ctx, products, ProductFieldOptions)
	return nil
}

// The templates a product's type hands it - taking one off the product itself loses nothing while the type has it
func (s *optionService) fromProductType(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]map[uuid.UUID]bool, error) {
	uses, err := s.repo.ListEffectiveOptionTemplates(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]map[uuid.UUID]bool, len(uses))
	for pid, list := range uses {
		for _, u := range list {
			if u.FromProductType {
				if out[pid] == nil {
					out[pid] = map[uuid.UUID]bool{}
				}
				out[pid][u.TemplateID] = true
			}
		}
	}
	return out, nil
}
