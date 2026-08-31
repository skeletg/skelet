package modelconv

import (
	"encoding/json"
	"sort"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/cms"
)

func unmarshalJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

func CMSRelationshipTarget(t cms.RelationshipTarget) model.CMSRelationshipTarget {
	switch t {
	case cms.TargetBlogPost:
		return model.CMSRelationshipTargetBlogPost
	case cms.TargetProduct:
		return model.CMSRelationshipTargetProduct
	case cms.TargetCollection:
		return model.CMSRelationshipTargetCollection
	case cms.TargetCategory:
		return model.CMSRelationshipTargetCategory
	}
	return model.CMSRelationshipTargetCmsPage
}

func CMSMetadataItems(m map[string]string) []*model.MetadataItem {
	if len(m) == 0 {
		return []*model.MetadataItem{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*model.MetadataItem, len(keys))
	for i, k := range keys {
		out[i] = &model.MetadataItem{Key: k, Value: m[k]}
	}
	return out
}

// ==================== Conversions ====================

func CMSPageContentModel(c cms.PageContent) *model.CMSPageContent {
	return &model.CMSPageContent{
		Locale:          c.Locale,
		Title:           c.Title,
		Body:            unmarshalJSON(c.Body),
		MetaTitle:       c.MetaTitle,
		MetaDescription: c.MetaDescription,
		MetaKeywords:    c.MetaKeywords,
		Data:            unmarshalJSON(c.Data),
		UpdatedAt:       c.UpdatedAt,
	}
}

func CMSPageContentModels(in []cms.PageContent) []*model.CMSPageContent {
	out := make([]*model.CMSPageContent, len(in))
	for i, c := range in {
		out[i] = CMSPageContentModel(c)
	}
	return out
}

func CMSPageSelectionsModel(in []cms.PageAttributeSelection) []*model.CMSPageSelection {
	out := make([]*model.CMSPageSelection, len(in))
	for i, s := range in {
		out[i] = &model.CMSPageSelection{
			AttributeID: s.AttributeID.String(),
			ValueID:     s.ValueID.String(),
		}
	}
	return out
}

func CMSPageRelationshipsModel(in []cms.PageRelationship) []*model.CMSPageRelationship {
	out := make([]*model.CMSPageRelationship, len(in))
	for i, rl := range in {
		out[i] = &model.CMSPageRelationship{
			AttributeID: rl.AttributeID.String(),
			TargetType:  CMSRelationshipTarget(rl.TargetType),
			TargetID:    rl.TargetID.String(),
			Position:    rl.Position,
		}
	}
	return out
}
