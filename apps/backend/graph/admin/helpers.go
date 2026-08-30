package admin

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/graph/modelconv"
)

func parseID(id, entity string) (uuid.UUID, error) { // the wrapped cause is safe to surface - uuid format only
	u, err := uuid.Parse(id)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("invalid %s id: %w", entity, err)
	}
	return u, nil
}

func parseOptionalUUID(s *string) *uuid.UUID {
	if s == nil {
		return nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil // gqlgen validates scalar types before resolvers, so a bad uuid shouldn't get this far
	}
	return &id
}

func metadataInputToStringMap(items []*model.MetadataInput) map[string]string { // nil and empty input both yield an empty, non-nil map
	out := make(map[string]string, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out[it.Key] = it.Value
	}
	return out
}

func jsonRawFromAny(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil // nil stays nil so callers can tell "no default" from "default = null"
	}
	return json.Marshal(v)
}

func marshalJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func boolOrDefault(p *bool, d bool) bool {
	if p == nil {
		return d
	}
	return *p
}

func stringOr(p *string, d string) string {
	if p == nil {
		return d
	}
	return *p
}

func stringOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func int32Or(p *int32, d int32) int32 {
	if p == nil {
		return d
	}
	return *p
}

func float64Or(p *float64, d float64) float64 {
	if p == nil {
		return d
	}
	return *p
}

// Shared converters live in graph/modelconv - these aliases keep existing call sites unchanged
var (
	toAddressInput            = modelconv.AddressInput
	toAddressModel            = modelconv.AddressModel
	toCategoryContentModel    = modelconv.CategoryContentModel
	toCollectionContentModels = modelconv.CollectionContentModels
	toCollectionCoverModel    = modelconv.CollectionCoverModel
	toConfigScopeModel        = modelconv.ConfigScopeModel
	toCurrencyModel           = modelconv.CurrencyModel
	toCustomerConsentModel    = modelconv.CustomerConsentModel
	toCustomerGroupModel      = modelconv.CustomerGroupModel
	toLocaleModel             = modelconv.LocaleModel
	toMetadataItems           = modelconv.MetadataItems
	toOrderAddressModel       = modelconv.OrderAddressModel
	toOrderEventModel         = modelconv.OrderEventModel
	toOrderLineModel          = modelconv.OrderLineModel
	toProductContentModels    = modelconv.ProductContentModels
	toProductMediaModel       = modelconv.ProductMediaModel
	toStringMapItems          = modelconv.StringMapItems
	attributeInputTypeToGQL   = modelconv.AttributeInputTypeToGQL
	referenceTargetToGQL      = modelconv.ReferenceTargetToGQL
	referenceTargetFromGQL    = modelconv.ReferenceTargetFromGQL
	boolOrFalse               = modelconv.BoolOrFalse
	customerMetadataItems     = modelconv.CustomerMetadataItems
	resolveMediaRole          = modelconv.ResolveMediaRole
	effectiveTracksInventory  = modelconv.EffectiveTracksInventory
	toGroupedMemberModels     = modelconv.GroupedMemberModels
	productVariantModeModel   = modelconv.ProductVariantModeModel

	toBlogAuthorModel   = modelconv.BlogAuthorModel
	toBlogCategoryModel = modelconv.BlogCategoryModel
	blogCategoryModels  = modelconv.BlogCategoryModels
	toBlogTagModel      = modelconv.BlogTagModel
	toBlogPostModel     = modelconv.BlogPostModel

	toCMSRelationshipTarget     = modelconv.CMSRelationshipTarget
	cmsMetadataItems            = modelconv.CMSMetadataItems
	toCMSPageContentModel       = modelconv.CMSPageContentModel
	toCMSPageContentModels      = modelconv.CMSPageContentModels
	toCMSPageSelectionsModel    = modelconv.CMSPageSelectionsModel
	toCMSPageRelationshipsModel = modelconv.CMSPageRelationshipsModel
)
