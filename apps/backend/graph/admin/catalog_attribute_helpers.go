package admin

import (
	"encoding/json"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/catalog"
)

func toAttributeModel(a catalog.Attribute) *model.Attribute {
	values := make([]*model.AttributeValue, len(a.Values))
	for i, v := range a.Values {
		values[i] = toAttributeValueModel(v)
	}
	var defaultValue any
	if len(a.DefaultValue) > 0 {
		_ = json.Unmarshal(a.DefaultValue, &defaultValue)
	}
	return &model.Attribute{
		ID:                 a.ID.String(),
		Slug:               a.Slug,
		Name:               a.Name,
		InputType:          attributeInputTypeToGQL(a.InputType),
		RelationshipTarget: referenceTargetToGQL(a.RelationshipTarget),
		Unit:               a.Unit,
		ValueRequired:      a.ValueRequired,
		Filterable:         a.Filterable,
		Searchable:         a.Searchable,
		IsPublic:           a.IsPublic,
		FacetCollapsed:     a.FacetCollapsed,
		IsIndexable:        a.IsIndexable,
		H1Position:         h1PositionToGQL(a.H1Position),
		SynapseTrack:       a.SynapseTrack,
		AutoSegment:        a.AutoSegment,
		SortOrder:          a.SortOrder,
		Values:             values,
		Metadata:           toMetadataItems(a.Metadata), // no Descriptions here - the Attribute.descriptions field resolver looks catalog_attribute_content up lazily
		PrivateMetadata:    toMetadataItems(a.PrivateMetadata),
		DefaultValue:       defaultValue,
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
	}
}

// Any unknown stored value reads as BEFORE, in case a hand-edited row slips past the DB CHECK constraint
func h1PositionToGQL(p catalog.H1Position) model.AttributeH1Position {
	if p == catalog.H1PositionAfter {
		return model.AttributeH1PositionAfter
	}
	return model.AttributeH1PositionBefore
}

func h1PositionFromGQL(p model.AttributeH1Position) catalog.H1Position {
	if p == model.AttributeH1PositionAfter {
		return catalog.H1PositionAfter
	}
	return catalog.H1PositionBefore
}

// The CreateAttribute variant - nil input collapses to BEFORE, matching the service-layer default
func h1PositionFromGQLPtr(p *model.AttributeH1Position) catalog.H1Position {
	if p == nil {
		return catalog.H1PositionBefore
	}
	return h1PositionFromGQL(*p)
}

func toProductAttributeSlugModel(s catalog.ProductAttributeSlug) *model.ProductAttributeSlug {
	m := &model.ProductAttributeSlug{
		ID:          s.ID.String(),
		ProductID:   s.ProductID.String(),
		AttributeID: s.AttributeID.String(),
		Slug:        s.Slug,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
	if s.ChannelID != nil {
		v := s.ChannelID.String()
		m.ChannelID = &v
	}
	return m
}

func toProductAttributeLocaleValueModel(v catalog.ProductAttributeLocaleValue) *model.ProductAttributeLocaleValue {
	return &model.ProductAttributeLocaleValue{
		ProductID:   v.ProductID.String(),
		AttributeID: v.AttributeID.String(),
		Locale:      v.Locale,
		Value:       v.Value,
		UpdatedAt:   v.UpdatedAt,
	}
}

func toAttributeValueChannelSlugModel(s catalog.AttributeValueChannelSlug) *model.AttributeValueChannelSlug {
	return &model.AttributeValueChannelSlug{
		ID:        s.ID.String(),
		ValueID:   s.ValueID.String(),
		ChannelID: s.ChannelID.String(),
		Slug:      s.Slug,
		UpdatedAt: s.UpdatedAt,
	}
}

func toAttributeValueModel(v catalog.AttributeValue) *model.AttributeValue {
	return &model.AttributeValue{
		ID:          v.ID.String(),
		Slug:        v.Slug,
		Name:        v.Name,
		Value:       v.Value,
		SortOrder:   v.SortOrder,
		IsIndexable: v.IsIndexable, // no Descriptions here - the field resolver fills them, keeping list views from fanning out to ListAttributeValueContent
	}
}

func toAttributeContentModel(c catalog.AttributeContent) *model.AttributeContent {
	return &model.AttributeContent{
		Locale:      c.Locale,
		Name:        c.Name,
		Description: c.Description,
		UpdatedAt:   c.UpdatedAt,
	}
}

func toAttributeValueContentModel(c catalog.AttributeValueContent) *model.AttributeValueContent {
	return &model.AttributeValueContent{
		Locale:      c.Locale,
		Name:        c.Name,
		Description: c.Description,
		UpdatedAt:   c.UpdatedAt,
	}
}

func attributeInputTypeFromGQL(t model.AttributeInputType) catalog.InputType {
	switch t {
	case model.AttributeInputTypePlainText:
		return catalog.InputTypePlainText
	case model.AttributeInputTypeNumeric:
		return catalog.InputTypeNumeric
	case model.AttributeInputTypeBoolean:
		return catalog.InputTypeBoolean
	case model.AttributeInputTypeDate:
		return catalog.InputTypeDate
	case model.AttributeInputTypeDateTime:
		return catalog.InputTypeDateTime
	case model.AttributeInputTypeDropdown:
		return catalog.InputTypeDropdown
	case model.AttributeInputTypeMultiselect:
		return catalog.InputTypeMultiselect
	case model.AttributeInputTypeSwatch:
		return catalog.InputTypeSwatch
	case model.AttributeInputTypeReference:
		return catalog.InputTypeReference
	}
	return catalog.InputTypePlainText
}

func attributeValueInputFromGQL(in *model.AttributeValueInput) catalog.AttributeValueInput {
	if in == nil {
		return catalog.AttributeValueInput{}
	}
	return catalog.AttributeValueInput{
		Slug:        in.Slug,
		Name:        in.Name,
		Value:       stringOr(in.Value, ""),
		SortOrder:   int32Or(in.SortOrder, 0),
		IsIndexable: boolOrFalse(in.IsIndexable),
	}
}
