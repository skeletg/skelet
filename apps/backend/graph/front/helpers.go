package front

import (
	"github.com/skeletg/skelet/apps/backend/graph/modelconv"
)

var ( // aliases onto graph/modelconv, the single source of truth - they keep existing call sites unchanged
	toAddressInput            = modelconv.AddressInput
	toAddressModel            = modelconv.AddressModel
	toCategoryContentModel    = modelconv.CategoryContentModel
	toCollectionContentModels = modelconv.CollectionContentModels
	toCollectionCoverModel    = modelconv.CollectionCoverModel
	toConfigScopeModel        = modelconv.ConfigScopeModel
	toConsentDefinitionModel  = modelconv.ConsentDefinitionModel
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
	boolOrFalse               = modelconv.BoolOrFalse
	customerMetadataItems     = modelconv.CustomerMetadataItems
	resolveMediaRole          = modelconv.ResolveMediaRole
	effectiveTracksInventory  = modelconv.EffectiveTracksInventory
	toGroupedMemberModels     = modelconv.GroupedMemberModels
	productVariantModeModel   = modelconv.ProductVariantModeModel

	toBlogAuthorModel      = modelconv.BlogAuthorModel
	toBlogCategoryModel    = modelconv.BlogCategoryModel
	blogCategoryModels     = modelconv.BlogCategoryModels
	toBlogTagModel         = modelconv.BlogTagModel
	toBlogPostModel        = modelconv.BlogPostModel
	postListedAndPublished = modelconv.PostListedAndPublished

	cmsMetadataItems            = modelconv.CMSMetadataItems
	toCMSPageContentModel       = modelconv.CMSPageContentModel
	toCMSPageContentModels      = modelconv.CMSPageContentModels
	toCMSPageSelectionsModel    = modelconv.CMSPageSelectionsModel
	toCMSPageRelationshipsModel = modelconv.CMSPageRelationshipsModel
)
