package catalog

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// GetGroupedMember returns the raw member row
func (s *groupedService) GetGroupedMember(ctx context.Context, id uuid.UUID) (GroupedMember, error) {
	return s.repo.GetGroupedMemberByID(ctx, id)
}

// AddGroupedMember attaches a SIMPLE product to a GROUPED parent - The parent's kind, the member's kind, and the "not self" rule are all validated here - Duplicates (same parent+member pair) are caught by the UNIQUE constraint and surfaced as ErrGroupedMemberDuplicate
func (s *groupedService) AddGroupedMember(ctx context.Context, input AddGroupedMemberInput) (GroupedMember, error) {
	if input.DefaultQuantity <= 0 {
		input.DefaultQuantity = 1
	}
	parent, err := s.repo.GetProductByID(ctx, input.ParentID)
	if err != nil {
		return GroupedMember{}, err
	}
	if parent.Kind != KindGrouped {
		return GroupedMember{}, fmt.Errorf("%w: product %q is kind %q", ErrGroupedParentNotGrouped, parent.SKU, string(parent.Kind))
	}
	if input.MemberID == parent.ID {
		return GroupedMember{}, fmt.Errorf("%w: %s", ErrGroupedMemberIsParent, parent.SKU)
	}
	member, err := s.repo.GetProductByID(ctx, input.MemberID)
	if err != nil {
		return GroupedMember{}, err
	}
	if member.Kind != KindSimple {
		return GroupedMember{}, fmt.Errorf("%w: product %q is kind %q", ErrGroupedMemberNotSimple, member.SKU, string(member.Kind))
	}
	return s.repo.CreateGroupedMember(ctx, input)
}

// UpdateGroupedMember applies pointer-style "no change = nil" updates
func (s *groupedService) UpdateGroupedMember(ctx context.Context, input UpdateGroupedMemberInput) (GroupedMember, error) {
	current, err := s.repo.GetGroupedMemberByID(ctx, input.ID)
	if err != nil {
		return GroupedMember{}, err
	}
	finalQuantity := current.DefaultQuantity
	if input.DefaultQuantity != nil {
		if *input.DefaultQuantity <= 0 {
			return GroupedMember{}, fmt.Errorf("default_quantity must be positive")
		}
		finalQuantity = *input.DefaultQuantity
	}
	finalSortOrder := current.SortOrder
	if input.SortOrder != nil {
		finalSortOrder = *input.SortOrder
	}
	return s.repo.UpdateGroupedMember(ctx, current.ID, finalQuantity, finalSortOrder)
}

// RemoveGroupedMember deletes a member by id
func (s *groupedService) RemoveGroupedMember(ctx context.Context, id uuid.UUID) error {
	if _, err := s.repo.GetGroupedMemberByID(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteGroupedMember(ctx, id)
}

// ListGroupedMembers loads every member for a grouped parent, hydrating each with the member product's display fields (slug, sku, name, price, currency) - Member names resolve to the requested locale with default-locale fallback, matching the bundle read path
// ListGroupedMemberLinksByMemberIDs batch-resolves the grouped products each product is a member of - Powers Product.partOf.groupedProducts
func (s *groupedService) ListGroupedMemberLinksByMemberIDs(ctx context.Context, memberIDs []uuid.UUID) (map[uuid.UUID][]GroupedMemberLink, error) {
	return s.repo.ListGroupedMemberLinksByMemberIDs(ctx, memberIDs)
}

func (s *groupedService) ListGroupedMembers(ctx context.Context, parentID uuid.UUID, locale string) ([]GroupedMember, error) {
	members, err := s.repo.ListGroupedMembersByParentID(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []GroupedMember{}, nil
	}

	defLocale := s.resolveDefaultLocale(ctx)
	if locale == "" {
		locale = defLocale
	}

	memberIDs := make([]uuid.UUID, 0, len(members))
	seen := make(map[uuid.UUID]struct{})
	for _, m := range members {
		if _, ok := seen[m.MemberID]; ok {
			continue
		}
		seen[m.MemberID] = struct{}{}
		memberIDs = append(memberIDs, m.MemberID)
	}

	productByID := make(map[uuid.UUID]Product, len(memberIDs))
	for _, pid := range memberIDs {
		p, err := s.repo.GetProductByID(ctx, pid)
		if err != nil {
			return nil, fmt.Errorf("load grouped member product %s: %w", pid, err)
		}
		productByID[pid] = p
	}
	contentByProduct, err := s.repo.ListProductContentByScope(ctx, memberIDs, locale)
	if err != nil {
		return nil, fmt.Errorf("load grouped member content: %w", err)
	}
	if locale != defLocale && defLocale != "" {
		fallback, err := s.repo.ListProductContentByScope(ctx, memberIDs, defLocale)
		if err != nil {
			return nil, fmt.Errorf("load grouped member fallback content: %w", err)
		}
		for pid, c := range fallback {
			if _, ok := contentByProduct[pid]; !ok {
				contentByProduct[pid] = c
			}
		}
	}

	for i := range members {
		m := &members[i]
		if p, ok := productByID[m.MemberID]; ok {
			m.MemberURL = p.URL
			m.MemberSKU = p.SKU
			m.MemberDefaultPrice = p.DefaultPrice
			m.MemberCurrency = p.DefaultCurrencyCode
			m.MemberMinOrderQty = p.MinOrderQty
			m.MemberMaxOrderQty = p.MaxOrderQty
			m.MemberQuantityStep = p.QuantityStep
			m.MemberQuantityDecimals = p.QuantityDecimals
			if c, ok := contentByProduct[p.ID]; ok {
				m.MemberName = c.Name
			}
		}
	}

	return members, nil
}
