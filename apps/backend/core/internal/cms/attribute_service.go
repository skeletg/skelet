package cms

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type attributeStore interface {
	AssignAttributeToPageType(ctx context.Context, in AssignAttributeInput) error
	CreateAttribute(ctx context.Context, in CreateAttributeInput) (Attribute, error)
	CreateAttributeGroup(ctx context.Context, slug string, position int32) (AttributeGroup, error)
	CreateAttributeValue(ctx context.Context, attributeID uuid.UUID, in AttributeValueInput) (AttributeValue, error)
	DeleteAttribute(ctx context.Context, id uuid.UUID) error
	DeleteAttributeGroup(ctx context.Context, id uuid.UUID) error
	DeleteAttributeValue(ctx context.Context, id uuid.UUID) error
	GetAttributeByID(ctx context.Context, id uuid.UUID) (Attribute, error)
	GetAttributeValueByID(ctx context.Context, id uuid.UUID) (AttributeValue, error)
	GetPageTypeByID(ctx context.Context, id uuid.UUID) (PageType, error)
	GetPageTypeWithAttributes(ctx context.Context, id uuid.UUID) (PageType, error)
	ListAttributeGroups(ctx context.Context) ([]AttributeGroup, error)
	ListAttributes(ctx context.Context) ([]Attribute, error)
	ListPageTypesUsingAttribute(ctx context.Context, attributeID uuid.UUID) ([]uuid.UUID, error)
	UnassignAttributeFromPageType(ctx context.Context, pageTypeID, attributeID uuid.UUID) error
	UpdateAttribute(ctx context.Context, in UpdateAttributeInput) (Attribute, error)
	UpdateAttributeGroup(ctx context.Context, id uuid.UUID, slug string, position int32) (AttributeGroup, error)
	UpdateAttributeValue(ctx context.Context, id uuid.UUID, in AttributeValueInput) (AttributeValue, error)
	UpsertAttributeContent(ctx context.Context, attributeID uuid.UUID, in AttributeContentInput) (AttributeContent, error)
	UpsertAttributeValueContent(ctx context.Context, valueID uuid.UUID, locale, name string) error
}

func (s *attributeService) CreateAttribute(ctx context.Context, in CreateAttributeInput) (Attribute, error) {
	if err := validateSlug(in.Slug); err != nil {
		return Attribute{}, err
	}
	if !in.InputType.IsValid() {
		return Attribute{}, fmt.Errorf("%w: %q", ErrInvalidInputType, in.InputType)
	}
	if in.InputType.IsRelationship() {
		if in.RelationshipTarget == nil || !in.RelationshipTarget.IsValid() {
			return Attribute{}, ErrInvalidRelationship
		}
	}
	if len(in.Values) > 0 && !in.InputType.AcceptsValues() {
		return Attribute{}, ErrValuesNotApplicable
	}
	for _, v := range in.Values {
		if err := validateSlug(v.Slug); err != nil {
			return Attribute{}, fmt.Errorf("attribute value: %w", err)
		}
	}
	return s.repo.CreateAttribute(ctx, in)
}

func (s *attributeService) GetAttribute(ctx context.Context, id uuid.UUID) (Attribute, error) {
	return s.repo.GetAttributeByID(ctx, id)
}

func (s *attributeService) ListAttributes(ctx context.Context) ([]Attribute, error) {
	return s.repo.ListAttributes(ctx)
}

func (s *attributeService) UpdateAttribute(ctx context.Context, in UpdateAttributeInput) (Attribute, error) {
	return s.repo.UpdateAttribute(ctx, in)
}

func (s *attributeService) DeleteAttribute(ctx context.Context, id uuid.UUID) error {
	used, err := s.repo.ListPageTypesUsingAttribute(ctx, id)
	if err != nil {
		return err
	}
	if len(used) > 0 {
		return ErrAttributeInUse
	}
	return s.repo.DeleteAttribute(ctx, id)
}

func (s *attributeService) UpsertAttributeContent(ctx context.Context, attributeID uuid.UUID, in AttributeContentInput) (Attribute, error) {
	if strings.TrimSpace(in.Locale) == "" {
		return Attribute{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return Attribute{}, fmt.Errorf("name is required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return Attribute{}, err
		}
	}
	if _, err := s.repo.UpsertAttributeContent(ctx, attributeID, in); err != nil {
		return Attribute{}, err
	}
	return s.repo.GetAttributeByID(ctx, attributeID)
}

func (s *attributeService) AddAttributeValue(ctx context.Context, attributeID uuid.UUID, in AttributeValueInput) (AttributeValue, error) {
	if err := validateSlug(in.Slug); err != nil {
		return AttributeValue{}, err
	}
	a, err := s.repo.GetAttributeByID(ctx, attributeID)
	if err != nil {
		return AttributeValue{}, err
	}
	if !a.InputType.AcceptsValues() {
		return AttributeValue{}, ErrValuesNotApplicable
	}
	return s.repo.CreateAttributeValue(ctx, attributeID, in)
}

func (s *attributeService) UpdateAttributeValue(ctx context.Context, id uuid.UUID, in AttributeValueInput) (AttributeValue, error) {
	if err := validateSlug(in.Slug); err != nil {
		return AttributeValue{}, err
	}
	return s.repo.UpdateAttributeValue(ctx, id, in)
}

func (s *attributeService) DeleteAttributeValue(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteAttributeValue(ctx, id)
}

// Returns the parent attribute with values and content, for the caller
func (s *attributeService) UpsertAttributeValueContent(ctx context.Context, valueID uuid.UUID, locale, name string) (Attribute, error) {
	if strings.TrimSpace(locale) == "" || strings.TrimSpace(name) == "" {
		return Attribute{}, fmt.Errorf("locale and name are required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, locale); err != nil {
			return Attribute{}, err
		}
	}
	v, err := s.repo.GetAttributeValueByID(ctx, valueID)
	if err != nil {
		return Attribute{}, err
	}
	if err := s.repo.UpsertAttributeValueContent(ctx, valueID, locale, name); err != nil {
		return Attribute{}, err
	}
	return s.repo.GetAttributeByID(ctx, v.AttributeID)
}

func (s *attributeService) CreateAttributeGroup(ctx context.Context, slug string, position int32) (AttributeGroup, error) {
	if err := validateSlug(slug); err != nil {
		return AttributeGroup{}, err
	}
	return s.repo.CreateAttributeGroup(ctx, slug, position)
}

func (s *attributeService) UpdateAttributeGroup(ctx context.Context, id uuid.UUID, slug string, position int32) (AttributeGroup, error) {
	if err := validateSlug(slug); err != nil {
		return AttributeGroup{}, err
	}
	return s.repo.UpdateAttributeGroup(ctx, id, slug, position)
}

func (s *attributeService) DeleteAttributeGroup(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteAttributeGroup(ctx, id)
}

func (s *attributeService) ListAttributeGroups(ctx context.Context) ([]AttributeGroup, error) {
	return s.repo.ListAttributeGroups(ctx)
}

func (s *attributeService) AssignAttribute(ctx context.Context, in AssignAttributeInput) (PageType, error) {
	if _, err := s.repo.GetPageTypeByID(ctx, in.PageTypeID); err != nil {
		return PageType{}, err
	}
	if _, err := s.repo.GetAttributeByID(ctx, in.AttributeID); err != nil {
		return PageType{}, err
	}
	if err := s.repo.AssignAttributeToPageType(ctx, in); err != nil {
		return PageType{}, err
	}
	return s.repo.GetPageTypeWithAttributes(ctx, in.PageTypeID)
}

func (s *attributeService) UnassignAttribute(ctx context.Context, pageTypeID, attributeID uuid.UUID) (PageType, error) {
	if err := s.repo.UnassignAttributeFromPageType(ctx, pageTypeID, attributeID); err != nil {
		return PageType{}, err
	}
	return s.repo.GetPageTypeWithAttributes(ctx, pageTypeID)
}
