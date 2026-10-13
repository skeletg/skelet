package catalog

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func (s *kitService) AddKitComponent(ctx context.Context, input AddKitComponentInput) (KitComponent, error) {
	if input.ProductID == input.ComponentProductID {
		return KitComponent{}, ErrKitComponentIsParent
	}
	if err := validPositiveQuantity(input.Quantity); err != nil {
		return KitComponent{}, err
	}
	parent, err := s.repo.GetProductByID(ctx, input.ProductID)
	if err != nil {
		return KitComponent{}, err
	}
	if parent.Kind != KindKit {
		return KitComponent{}, fmt.Errorf("%w: product %q is kind %q", ErrKitParentNotKit, parent.SKU, string(parent.Kind))
	}
	component, err := s.repo.GetProductByID(ctx, input.ComponentProductID)
	if err != nil {
		return KitComponent{}, err
	}
	if component.Kind != KindSimple {
		return KitComponent{}, fmt.Errorf("%w: %q is kind %q", ErrKitComponentNotSimple, component.SKU, string(component.Kind))
	}
	return s.repo.CreateKitComponent(ctx, input)
}

// Hydrates each component product's display fields for admin + storefront reads - the inventory expansion path uses KitComponentRefs, which skips them
func (s *kitService) ListKitComponents(ctx context.Context, productID uuid.UUID, locale string) ([]KitComponent, error) {
	comps, err := s.repo.ListKitComponentsByProductID(ctx, productID)
	if err != nil {
		return nil, err
	}
	ids := uniqueIDs(len(comps), func(i int) uuid.UUID { return comps[i].ComponentProductID })
	products, content, err := s.loadProductsWithContent(ctx, ids, locale, true)
	if err != nil {
		return nil, fmt.Errorf("kit components: %w", err)
	}
	for i := range comps {
		p := products[comps[i].ComponentProductID]
		comps[i].ComponentURL = p.URL
		comps[i].ComponentSKU = p.SKU
		comps[i].ComponentDefaultPrice = p.DefaultPrice
		comps[i].ComponentCurrency = p.DefaultCurrencyCode
		comps[i].ComponentName = content[p.ID].Name
	}
	return comps, nil
}

// Bare rows, no product hydration - the fast path for the inventory bridge, which only needs component product id + quantity to build reservations
func (s *kitService) KitComponentRefs(ctx context.Context, productID uuid.UUID) ([]KitComponent, error) {
	return s.repo.ListKitComponentsByProductID(ctx, productID)
}

func (s *kitService) UpdateKitComponent(ctx context.Context, input UpdateKitComponentInput) (KitComponent, error) {
	current, err := s.repo.GetKitComponentByID(ctx, input.ID)
	if err != nil {
		return KitComponent{}, err
	}
	quantity := current.Quantity
	if input.Quantity != nil {
		if err := validPositiveQuantity(*input.Quantity); err != nil {
			return KitComponent{}, err
		}
		quantity = *input.Quantity
	}
	sortOrder := current.SortOrder
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}
	return s.repo.UpdateKitComponent(ctx, current.ID, quantity, sortOrder)
}

func (s *kitService) GetKitComponent(ctx context.Context, id uuid.UUID) (KitComponent, error) {
	return s.repo.GetKitComponentByID(ctx, id)
}

func (s *kitService) DeleteKitComponent(ctx context.Context, id uuid.UUID) error {
	if _, err := s.repo.GetKitComponentByID(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteKitComponent(ctx, id)
}
