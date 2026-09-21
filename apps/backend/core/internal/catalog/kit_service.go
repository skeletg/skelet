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
	if input.Quantity <= 0 {
		input.Quantity = 1
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

// Hydrates each component product's display fields (slug/sku/price) for admin + storefront reads - the inventory expansion path uses KitComponentRefs instead, which skips the per-component loads
func (s *kitService) ListKitComponents(ctx context.Context, productID uuid.UUID) ([]KitComponent, error) {
	comps, err := s.repo.ListKitComponentsByProductID(ctx, productID)
	if err != nil {
		return nil, err
	}
	for i := range comps {
		p, err := s.repo.GetProductByID(ctx, comps[i].ComponentProductID)
		if err != nil {
			continue // leave display fields empty on a lookup miss
		}
		comps[i].ComponentURL = p.URL
		comps[i].ComponentSKU = p.SKU
		comps[i].ComponentDefaultPrice = p.DefaultPrice
		comps[i].ComponentCurrency = p.DefaultCurrencyCode
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
		if *input.Quantity <= 0 {
			return KitComponent{}, fmt.Errorf("quantity must be positive")
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
