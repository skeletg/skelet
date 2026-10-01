package catalog

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

// Repository is the persistence contract for the catalog domain, satisfied by *postgresRepository
// It exists as a seam so the service layer can be tested against an in-memory fake (see service_test.go); production wiring still passes the concrete Postgres implementation - WithTx returns Repository so transactional call sites compose against the interface
var _ Repository = (*postgresRepository)(nil)

type Repository interface {
	WithTx(t *sql.Tx) Repository

	// Product types, attribute groups, and the wiring that assigns attributes to types
	AssignAttribute(ctx context.Context, input AssignAttributeInput) error
	CountProductsUsingConfigurableOptionForType(ctx context.Context, attributeID, productTypeID uuid.UUID) (int64, error)
	CreateAttributeGroup(ctx context.Context, input CreateAttributeGroupInput) (AttributeGroup, error)
	CreateProductType(ctx context.Context, input CreateProductTypeInput) (ProductType, error)
	DeleteAttributeGroup(ctx context.Context, id uuid.UUID) error
	DeleteProductType(ctx context.Context, id uuid.UUID) error
	GetAttributeGroupByID(ctx context.Context, id uuid.UUID) (AttributeGroup, error)
	GetProductTypeByID(ctx context.Context, id uuid.UUID) (ProductType, error)
	GetProductTypeWithAttributes(ctx context.Context, id uuid.UUID) (ProductType, error)
	ListAttributeGroupsByProductType(ctx context.Context, productTypeID uuid.UUID) ([]AttributeGroup, error)
	ListProductTypes(ctx context.Context) ([]ProductType, error)
	MoveAttributeToGroup(ctx context.Context, productTypeID, attributeID uuid.UUID, groupID *uuid.UUID) error
	UnassignAttribute(ctx context.Context, productTypeID, attributeID uuid.UUID) error
	UpdateAttributeGroup(ctx context.Context, input UpdateAttributeGroupInput) (AttributeGroup, error)
	UpdateProductType(ctx context.Context, input UpdateProductTypeInput) (ProductType, error)

	// Attributes, attribute values, their localized content, and channel slugs
	AddAttributeValue(ctx context.Context, attributeID uuid.UUID, v AttributeValueInput) (AttributeValue, error)
	CascadeAttributeIndexable(ctx context.Context, attributeID uuid.UUID, isIndexable bool) (CascadeAttributeIndexableResult, error)
	SetAttributeValuesIndexable(ctx context.Context, valueIDs []uuid.UUID, isIndexable bool) (int64, error)
	CountProductsUsingConfigurableOption(ctx context.Context, attributeID uuid.UUID) (int64, error)
	CreateAttribute(ctx context.Context, input CreateAttributeInput) (Attribute, error)
	DeleteAttribute(ctx context.Context, id uuid.UUID) error
	DeleteAttributeContent(ctx context.Context, attributeID uuid.UUID, locale string) error
	DeleteAttributeValue(ctx context.Context, id uuid.UUID) error
	DeleteAttributeValueChannelSlug(ctx context.Context, valueID, channelID uuid.UUID) error
	DeleteAttributeValueContent(ctx context.Context, valueID uuid.UUID, locale string) error
	GetAttributeByID(ctx context.Context, id uuid.UUID) (Attribute, error)
	GetAttributeValueByID(ctx context.Context, id uuid.UUID) (AttributeValue, error)
	GetAttributeWithValues(ctx context.Context, id uuid.UUID) (Attribute, error)
	ListAttributeContent(ctx context.Context, attributeID uuid.UUID) ([]AttributeContent, error)
	ListAttributeContentByAttributeIDs(ctx context.Context, attributeIDs []uuid.UUID) ([]AttributeContent, error)
	ListAttributeValueChannelSlugsByValueIDs(ctx context.Context, valueIDs []uuid.UUID) ([]AttributeValueChannelSlug, error)
	ListAttributeValueChannelSlugsForChannel(ctx context.Context, channelID uuid.UUID) ([]AttributeValueChannelSlug, error)
	ListAttributeValueContent(ctx context.Context, valueID uuid.UUID) ([]AttributeValueContent, error)
	ListAttributeValueContentByValueIDs(ctx context.Context, valueIDs []uuid.UUID) ([]AttributeValueContent, error)
	ListAttributes(ctx context.Context) ([]Attribute, error)
	ListProductIDsByAttributeValue(ctx context.Context, valueID uuid.UUID, tokenPattern string) ([]uuid.UUID, error)
	UpdateAttribute(ctx context.Context, input UpdateAttributeInput) (Attribute, error)
	UpdateAttributeValue(ctx context.Context, id uuid.UUID, v AttributeValueInput) (AttributeValue, error)
	UpsertAttributeContent(ctx context.Context, input UpsertAttributeContentInput) (AttributeContent, error)
	UpsertAttributeValueChannelSlug(ctx context.Context, valueID, attributeID, channelID uuid.UUID, slug string) (AttributeValueChannelSlug, error)
	UpsertAttributeValueContent(ctx context.Context, input UpsertAttributeValueContentInput) (AttributeValueContent, error)

	// Product CRUD, parent/child relations, variants, and configurable-option wiring
	AddProductChild(ctx context.Context, parentID, childID uuid.UUID, sortOrder int32) error
	CreateProduct(ctx context.Context, input CreateProductInput) (Product, error)
	DeleteProduct(ctx context.Context, id uuid.UUID) error
	GetProductByID(ctx context.Context, id uuid.UUID) (Product, error)
	GetProductBySKU(ctx context.Context, sku string) (Product, error)
	GetProductsBySKUs(ctx context.Context, skus []string) (map[string]Product, error)
	GetProductWithAttributes(ctx context.Context, id uuid.UUID) (Product, error)
	HydrateProductGraph(ctx context.Context, p Product) (Product, error)
	ListConfigurableOptions(ctx context.Context, productID uuid.UUID) ([]Attribute, error)
	ListConfigurableOptionsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]Attribute, error)
	ListConfigurableParentByChildIDs(ctx context.Context, childIDs []uuid.UUID) (map[uuid.UUID]ConfigurableParentLink, error)
	ListProductChildren(ctx context.Context, parentID uuid.UUID) ([]Product, error)
	ListProductChildrenByParentIDs(ctx context.Context, parentIDs []uuid.UUID) (map[uuid.UUID][]Product, error)
	ListProductChildIDsByParentIDs(ctx context.Context, parentIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	ListProductTypeIDsByProductIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]uuid.UUID, error)
	ListProductRowsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Product, error)
	ListProductContentByProductIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]ProductContent, error)
	ListProductParents(ctx context.Context, childID uuid.UUID) ([]Product, error)
	ListProductSelectableValuesByIDs(ctx context.Context, productIDs, attributeIDs []uuid.UUID) (map[uuid.UUID]map[uuid.UUID][]uuid.UUID, error)
	CountActiveCartReferencesToSKU(ctx context.Context, sku string) (int64, error)
	CountInheritingListingsWithCurrencyMismatchForProduct(ctx context.Context, productID uuid.UUID, currency string) (int64, error)
	UpsertProductChannelListingPublished(ctx context.Context, productID, channelID uuid.UUID, published bool) (ProductChannelListing, error)
	ListReadyProductMediaByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductMedia, error)
	ListProductIDsCarryingAttribute(ctx context.Context, attributeID uuid.UUID, slug string) ([]uuid.UUID, error)
	ListProductIDsCarryingAttributeValue(ctx context.Context, valueID uuid.UUID) ([]uuid.UUID, error)
	ListProducts(ctx context.Context, rowLimit int32) ([]Product, error) // rowLimit 0 means every row - see the query comment
	ListProductsForChannel(ctx context.Context, channelID uuid.UUID, rowLimit int32) ([]Product, error)
	ListProductIDsPaged(ctx context.Context, f ProductPageFilter) ([]uuid.UUID, int64, error)
	CountProductsInCategory(ctx context.Context, categoryID uuid.UUID) (int64, error)
	ListProductsByCollection(ctx context.Context, collectionID uuid.UUID, limit int32) ([]Product, error)
	ListProductsByCategory(ctx context.Context, categoryID uuid.UUID, limit int32) ([]Product, error)
	ListProductsByType(ctx context.Context, productTypeID uuid.UUID) ([]Product, error)
	ListProductsWithAttributesByIDs(ctx context.Context, ids []uuid.UUID) ([]Product, error)
	// Postgres listing path used when the search index is unavailable
	BrowseFallback(ctx context.Context, params db.BrowseProductsFallbackParams) ([]db.BrowseProductsFallbackRow, error)
	SearchFallback(ctx context.Context, params db.SearchProductsFallbackParams) ([]db.SearchProductsFallbackRow, error)
	FacetCounts(ctx context.Context, params db.BrowseFacetCountsFallbackParams) ([]db.BrowseFacetCountsFallbackRow, error)
	ProductTaxClassesBySKUs(ctx context.Context, skus []string) (map[string]string, error)
	ProductCostsBySKUs(ctx context.Context, skus []string) (map[string]int64, error)
	ProductDimensionsBySKUs(ctx context.Context, skus []string) (map[string]Dimensions, error)
	RemoveProductChild(ctx context.Context, parentID, childID uuid.UUID) error
	SetConfigurableOptions(ctx context.Context, productID uuid.UUID, options []AssignAttributeInput) error
	SetProductTaxClass(ctx context.Context, id uuid.UUID, taxClass string) (Product, error)
	SetProductVariantMode(ctx context.Context, id uuid.UUID, mode VariantMode) (Product, error)
	UpdateProduct(ctx context.Context, input UpdateProductInput) (Product, error)

	// Per-product attribute selections, locale values, and slugs
	DeleteProductAttributeLocaleValue(ctx context.Context, productID, attributeID uuid.UUID, locale string) error
	DeleteProductAttributeLocaleValuesForAttribute(ctx context.Context, productID, attributeID uuid.UUID) error
	DeleteProductAttributeSlug(ctx context.Context, productID, attributeID uuid.UUID, channelID *uuid.UUID) error
	ListProductAttributeLocaleValues(ctx context.Context, productID uuid.UUID) ([]ProductAttributeLocaleValue, error)
	ListProductAttributeLocaleValuesByProductIDs(ctx context.Context, productIDs []uuid.UUID) ([]ProductAttributeLocaleValue, error)
	ListProductAttributeSlugs(ctx context.Context, productID uuid.UUID) ([]ProductAttributeSlug, error)
	ListProductAttributeSlugsByProductIDs(ctx context.Context, productIDs []uuid.UUID) ([]ProductAttributeSlug, error)
	LoadProductsAttributeSelections(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]ProductAttributeSelection, error)
	LoadProductsAttributeReferences(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]ProductAttributeReference, error)
	SetProductAttributes(ctx context.Context, productID uuid.UUID, scalars ProductScalarAttributes, selections []ProductAttributeSelection) error
	SetProductAttributeReferences(ctx context.Context, productID, attributeID uuid.UUID, refs []ProductAttributeReference) error
	UpsertProductAttributeLocaleValue(ctx context.Context, input ProductAttributeLocaleValue) (ProductAttributeLocaleValue, error)
	UpsertProductAttributeSlug(ctx context.Context, input UpsertProductAttributeSlugInput) (ProductAttributeSlug, error)

	// Localized product content
	DeleteProductContent(ctx context.Context, productID uuid.UUID, locale string) error
	GetProductContent(ctx context.Context, productID uuid.UUID, locale string) (ProductContent, error)
	ListProductContent(ctx context.Context, productID uuid.UUID) ([]ProductContent, error)
	ListProductContentByScope(ctx context.Context, productIDs []uuid.UUID, locale string) (map[uuid.UUID]ProductContent, error)
	ListProductContentForProducts(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductContent, error)
	UpsertProductContent(ctx context.Context, productID uuid.UUID, input ProductContentInput) (ProductContent, error)

	// Product channel listings and URL paths
	CountInheritingListingsForChannel(ctx context.Context, channelID uuid.UUID) (int64, error)
	DeleteProductChannelListing(ctx context.Context, productID, channelID uuid.UUID) error
	GetProductChannelListing(ctx context.Context, productID, channelID uuid.UUID) (ProductChannelListing, error)
	ListProductChannelListingsByChannel(ctx context.Context, channelID uuid.UUID) ([]ProductChannelListing, error)
	ListProductChannelListingsByProduct(ctx context.Context, productID uuid.UUID) ([]ProductChannelListing, error)
	ListProductChannelListingsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductChannelListing, error)
	ListProductChannelListingsByProductIDsForChannel(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID][]ProductChannelListing, error)
	UpdateProductURL(ctx context.Context, id uuid.UUID, url string) error
	UpsertProductChannelListing(ctx context.Context, input ProductChannelListingInput) (ProductChannelListing, error)
	// Tier (customer-group / quantity-break) prices
	ListGroupPrices(ctx context.Context, productID uuid.UUID) ([]GroupPrice, error)
	ReplaceGroupPrices(ctx context.Context, productID uuid.UUID, inputs []GroupPriceInput) ([]GroupPrice, error)
	// Curated links + also-bought recommendations
	ListProductLinks(ctx context.Context, productID uuid.UUID, kind LinkKind) ([]uuid.UUID, error)
	ReplaceProductLinks(ctx context.Context, productID uuid.UUID, kind LinkKind, linked []uuid.UUID) error
	ListAlsoBought(ctx context.Context, productID uuid.UUID, limit int32) ([]uuid.UUID, error)
	RebuildAlsoBought(ctx context.Context) error

	// Categories: tree, membership, content, channel listings, cover, and storefront/published reads
	AddProductToCategory(ctx context.Context, productID, categoryID uuid.UUID, position int32, isPrimary bool) error
	ClearCategoryCover(ctx context.Context, categoryID uuid.UUID) (Category, error)
	ConfirmCategoryCover(ctx context.Context, categoryID, coverID uuid.UUID, width, height int32) (Category, error)
	RewriteCategorySubtreeURLPaths(ctx context.Context, id uuid.UUID, urlPath string) ([]CategoryPathMove, error)
	CategorySubtreeHeight(ctx context.Context, id uuid.UUID) (int32, error)
	LockCategoryTree(ctx context.Context) error
	LockCategoryByID(ctx context.Context, id uuid.UUID) (Category, error)
	CountProductsByCategoryIDs(ctx context.Context, categoryIDs []uuid.UUID) (map[uuid.UUID]int64, error)
	CreateCategory(ctx context.Context, input CreateCategoryInput) (Category, error)
	DeleteCategory(ctx context.Context, id uuid.UUID) error
	DeleteCategoryChannelListing(ctx context.Context, categoryID, channelID uuid.UUID) error
	DeleteCategoryContent(ctx context.Context, categoryID uuid.UUID, locale string) error
	FilterFullyVisibleCategoryIDsForChannel(ctx context.Context, ids []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]struct{}, error)
	FilterPublishedCategoryIDsForChannel(ctx context.Context, ids []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]struct{}, error)
	GetCategoryAncestors(ctx context.Context, id uuid.UUID) ([]Category, error)
	GetCategoryByID(ctx context.Context, id uuid.UUID) (Category, error)
	GetCategoryBySlug(ctx context.Context, slug string) (Category, error)
	GetCategoryChannelListing(ctx context.Context, categoryID, channelID uuid.UUID) (CategoryChannelListing, error)
	GetCategoryContent(ctx context.Context, categoryID uuid.UUID, locale string) (CategoryContent, error)
	GetPublishedCategoryForChannel(ctx context.Context, channelID, categoryID uuid.UUID) (Category, error)
	ListAllCategories(ctx context.Context) ([]Category, error)
	ListCategoriesByProduct(ctx context.Context, productID uuid.UUID) ([]Category, error)
	ListCategoriesByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]Category, error)
	ListCategoryChannelListings(ctx context.Context, categoryID uuid.UUID) ([]CategoryChannelListing, error)
	ListCategoryFacetOverrides(ctx context.Context, categoryID uuid.UUID) ([]FacetOverride, error)
	ReplaceCategoryFacetOverrides(ctx context.Context, categoryID uuid.UUID, overrides []FacetOverride) error
	ListCategoryChildren(ctx context.Context, parentID uuid.UUID) ([]Category, error)
	ListCategoriesListedOnChannel(ctx context.Context, channelID uuid.UUID) ([]Category, error)
	ListCategoryChildrenListedOnChannel(ctx context.Context, channelID, parentID uuid.UUID) ([]Category, error)
	ListCategoryContent(ctx context.Context, categoryID uuid.UUID) ([]CategoryContent, error)
	ListCategoryContentByScope(ctx context.Context, categoryIDs []uuid.UUID, locale string) (map[uuid.UUID]CategoryContent, error)
	ListCategoryContentForCategories(ctx context.Context, categoryIDs []uuid.UUID) (map[uuid.UUID][]CategoryContent, error)
	ListPrimaryCategoryByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]*Category, error)
	ListPublishedCategoriesForChannel(ctx context.Context, channelID uuid.UUID) ([]Category, error)
	ListPublishedCategoryChildrenForChannel(ctx context.Context, channelID, parentID uuid.UUID) ([]Category, error)
	ListPublishedRootCategoriesForChannel(ctx context.Context, channelID uuid.UUID) ([]Category, error)
	ListRootCategories(ctx context.Context) ([]Category, error)
	RemoveProductFromCategory(ctx context.Context, productID, categoryID uuid.UUID) error
	SetCategoryCover(ctx context.Context, categoryID, coverID uuid.UUID, s3Key, mimeType string, sizeBytes int64, altText string) (Category, error)
	SetPrimaryProductCategory(ctx context.Context, productID, categoryID uuid.UUID) error
	UpdateCategory(ctx context.Context, input UpdateCategoryInput) (Category, error)
	UpdateCategoryURLPath(ctx context.Context, id uuid.UUID, urlPath string) error
	UpsertCategoryChannelListing(ctx context.Context, in SetCategoryChannelListingInput) (CategoryChannelListing, error)
	UpsertCategoryContent(ctx context.Context, categoryID uuid.UUID, input CategoryContentInput) (CategoryContent, error)

	// Collections: CRUD, content, cover, and membership
	AddProductToCollection(ctx context.Context, collectionID, productID uuid.UUID, sortOrder int32) error
	ClearCollectionCover(ctx context.Context, collectionID uuid.UUID) (Collection, error)
	ConfirmCollectionCover(ctx context.Context, collectionID, coverID uuid.UUID, width, height int32) (Collection, error)
	CreateCollection(ctx context.Context, input CreateCollectionInput) (Collection, error)
	DeleteCollection(ctx context.Context, id uuid.UUID) error
	DeleteCollectionContent(ctx context.Context, collectionID uuid.UUID, locale string) error
	GetCollectionByID(ctx context.Context, id uuid.UUID) (Collection, error)
	GetCollectionContent(ctx context.Context, collectionID uuid.UUID, locale string) (CollectionContent, error)
	ListCollectionContent(ctx context.Context, collectionID uuid.UUID) ([]CollectionContent, error)
	ListCollectionContentForCollections(ctx context.Context, collectionIDs []uuid.UUID) (map[uuid.UUID][]CollectionContent, error)
	ListCollectionContentByScope(ctx context.Context, collectionIDs []uuid.UUID, locale string) (map[uuid.UUID]CollectionContent, error)
	ListCollectionFacetOverrides(ctx context.Context, collectionID uuid.UUID) ([]FacetOverride, error)
	ReplaceCollectionFacetOverrides(ctx context.Context, collectionID uuid.UUID, overrides []FacetOverride) error
	ListCollections(ctx context.Context) ([]Collection, error)
	ListCollectionsByProduct(ctx context.Context, productID uuid.UUID) ([]Collection, error)
	ListCollectionsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]Collection, error)
	RemoveProductFromCollection(ctx context.Context, collectionID, productID uuid.UUID) error
	ReorderCollectionProduct(ctx context.Context, collectionID, productID uuid.UUID, sortOrder int32) error
	SetCollectionCover(ctx context.Context, collectionID, coverID uuid.UUID, s3Key, mimeType string, sizeBytes int64, altText string) (Collection, error)
	UpdateCollection(ctx context.Context, input UpdateCollectionInput) (Collection, error)
	UpsertCollectionContent(ctx context.Context, collectionID uuid.UUID, input CollectionContentInput) (CollectionContent, error)

	// Product options and their values, including localized content
	CountProductOptionValueDefaultsForOption(ctx context.Context, optionID uuid.UUID) (int64, error)
	CreateProductOption(ctx context.Context, input AddProductOptionInput) (ProductOption, error)
	CreateProductOptionValue(ctx context.Context, input AddProductOptionValueInput) (ProductOptionValue, error)
	DeleteProductOption(ctx context.Context, id uuid.UUID) error
	DeleteProductOptionContent(ctx context.Context, optionID uuid.UUID, locale string) error
	DeleteProductOptionValue(ctx context.Context, id uuid.UUID) error
	DeleteProductOptionValueContent(ctx context.Context, valueID uuid.UUID, locale string) error
	GetProductOptionByID(ctx context.Context, id uuid.UUID) (ProductOption, error)
	GetProductOptionByIDForUpdate(ctx context.Context, id uuid.UUID) (ProductOption, error)
	GetProductOptionValueByID(ctx context.Context, id uuid.UUID) (ProductOptionValue, error)
	ListProductOptionContent(ctx context.Context, optionID uuid.UUID) ([]ProductOptionContent, error)
	ListProductOptionContentByOptionIDs(ctx context.Context, optionIDs []uuid.UUID) (map[uuid.UUID][]ProductOptionContent, error)
	ListProductOptionValueContent(ctx context.Context, valueID uuid.UUID) ([]ProductOptionValueContent, error)
	ListProductOptionValueContentByValueIDs(ctx context.Context, valueIDs []uuid.UUID) (map[uuid.UUID][]ProductOptionValueContent, error)
	ListProductOptionValuesByOptionID(ctx context.Context, optionID uuid.UUID) ([]ProductOptionValue, error)
	ListProductOptionValuesByOptionIDs(ctx context.Context, optionIDs []uuid.UUID) (map[uuid.UUID][]ProductOptionValue, error)
	ListProductOptionsByProductID(ctx context.Context, productID uuid.UUID) ([]ProductOption, error)
	ListProductOptionsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductOption, error)
	UpdateProductOption(ctx context.Context, id uuid.UUID, params UpdateProductOptionFields) (ProductOption, error)
	UpdateProductOptionValue(ctx context.Context, id uuid.UUID, sortOrder, priceValue int32, priceType ProductOptionPriceType, isDefault bool, sourceProductID *uuid.UUID, quantity float64, forcedLineQty *float64) (ProductOptionValue, error)
	UpsertProductOptionContent(ctx context.Context, input UpsertProductOptionContentInput) (ProductOptionContent, error)
	UpsertProductOptionValueContent(ctx context.Context, input UpsertProductOptionValueContentInput) (ProductOptionValueContent, error)

	// Product media assets and media role assignments
	ClearMediaRole(ctx context.Context, productID uuid.UUID, role MediaRole) error
	CreateProductMedia(ctx context.Context, id, productID uuid.UUID, s3Key, mimeType string, sizeBytes int64, altText string, sortOrder int32) (ProductMedia, error)
	DeletePendingMediaOlderThan(ctx context.Context, before time.Time) ([]PendingMediaRow, error)
	DeleteProductMedia(ctx context.Context, id uuid.UUID) error
	DeleteProductMediaByIDs(ctx context.Context, productID uuid.UUID, ids []uuid.UUID) ([]PendingMediaRow, error)
	GetMediaRoles(ctx context.Context, productID uuid.UUID) (map[MediaRole]uuid.UUID, error)
	GetMediaRolesByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]map[MediaRole]uuid.UUID, error)
	GetProductMediaByID(ctx context.Context, id uuid.UUID) (ProductMedia, error)
	ListProductMediaByProduct(ctx context.Context, productID uuid.UUID) ([]ProductMedia, error)
	ListProductMediaByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductMedia, error)
	MarkProductMediaReady(ctx context.Context, id uuid.UUID, width, height int32) (ProductMedia, error)
	SetMediaRole(ctx context.Context, productID, mediaID uuid.UUID, role MediaRole) error
	SetProductMediaSortOrder(ctx context.Context, id uuid.UUID, order int32) error
	UpdateProductMediaMetadata(ctx context.Context, input UpdateProductMediaInput) (ProductMedia, error)

	// Bundle options and selections, including localized content
	CountBundleSelectionDefaultsForOption(ctx context.Context, optionID uuid.UUID) (int64, error)
	CreateBundleOption(ctx context.Context, input AddBundleOptionInput) (BundleOption, error)
	CreateBundleSelection(ctx context.Context, input AddBundleSelectionInput) (BundleSelection, error)
	DeleteBundleOption(ctx context.Context, id uuid.UUID) error
	DeleteBundleOptionContent(ctx context.Context, optionID uuid.UUID, locale string) error
	DeleteBundleSelection(ctx context.Context, id uuid.UUID) error
	GetBundleOptionByID(ctx context.Context, id uuid.UUID) (BundleOption, error)
	GetBundleOptionByIDForUpdate(ctx context.Context, id uuid.UUID) (BundleOption, error)
	GetBundleSelectionByID(ctx context.Context, id uuid.UUID) (BundleSelection, error)
	ListBundleOptionContent(ctx context.Context, optionID uuid.UUID) ([]BundleOptionContent, error)
	ListBundleOptionContentByOptionIDs(ctx context.Context, optionIDs []uuid.UUID) (map[uuid.UUID][]BundleOptionContent, error)
	ListBundleOptionsByProductID(ctx context.Context, productID uuid.UUID) ([]BundleOption, error)
	ListBundleSelectionLinksByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]BundleSelectionLink, error)
	ListBundleSelectionsByOptionIDs(ctx context.Context, optionIDs []uuid.UUID) (map[uuid.UUID][]BundleSelection, error)
	UpdateBundleOption(ctx context.Context, id uuid.UUID, inputType BundleInputType, isRequired bool, sortOrder, minSelections int32, maxSelections *int32) (BundleOption, error)
	UpdateBundleSelection(ctx context.Context, id uuid.UUID, quantity float64, isDefault bool, sortOrder int32) (BundleSelection, error)
	UpsertBundleOptionContent(ctx context.Context, input UpsertBundleOptionContentInput) (BundleOptionContent, error)

	// Grouped-product members
	CreateGroupedMember(ctx context.Context, input AddGroupedMemberInput) (GroupedMember, error)
	DeleteGroupedMember(ctx context.Context, id uuid.UUID) error
	GetGroupedMemberByID(ctx context.Context, id uuid.UUID) (GroupedMember, error)
	ListGroupedMemberLinksByMemberIDs(ctx context.Context, memberIDs []uuid.UUID) (map[uuid.UUID][]GroupedMemberLink, error)
	ListGroupedMembersByParentID(ctx context.Context, parentID uuid.UUID) ([]GroupedMember, error)
	UpdateGroupedMember(ctx context.Context, id uuid.UUID, defaultQuantity float64, sortOrder int32) (GroupedMember, error)

	// Kit components
	CreateKitComponent(ctx context.Context, input AddKitComponentInput) (KitComponent, error)
	DeleteKitComponent(ctx context.Context, id uuid.UUID) error
	GetKitComponentByID(ctx context.Context, id uuid.UUID) (KitComponent, error)
	ListKitComponentsByProductID(ctx context.Context, productID uuid.UUID) ([]KitComponent, error)
	UpdateKitComponent(ctx context.Context, id uuid.UUID, quantity float64, sortOrder int32) (KitComponent, error)

	// Digital product download definitions
	CreateProductDownload(ctx context.Context, input AddProductDownloadInput) (ProductDownload, error)
	DeleteProductDownload(ctx context.Context, id uuid.UUID) error
	GetProductDownloadByID(ctx context.Context, id uuid.UUID) (ProductDownload, error)
	ListProductDownloadsByProductID(ctx context.Context, productID uuid.UUID) ([]ProductDownload, error)
	UpdateProductDownload(ctx context.Context, id uuid.UUID, title, url string, maxDownloads *int32, sortOrder int32) (ProductDownload, error)

	// Per-product subscription terms
	DeleteSubscriptionTerms(ctx context.Context, productID uuid.UUID) error
	GetSubscriptionTerms(ctx context.Context, productID uuid.UUID) (SubscriptionTerms, error)
	UpsertSubscriptionTerms(ctx context.Context, input SetSubscriptionTermsInput) (SubscriptionTerms, error)

	// Product storefront/published reads and pretty-URL resolution
	FilterPublishedProductIDsForChannel(ctx context.Context, candidateIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]struct{}, error)
	GetListingProductIDByChannelURL(ctx context.Context, channelID uuid.UUID, url string) (uuid.UUID, error)
	GetPublishedProductForChannel(ctx context.Context, id, channelID uuid.UUID) (Product, error)
	ListProductIDsInPrimaryCategoryDescendants(ctx context.Context, rootID uuid.UUID) ([]uuid.UUID, error)
}
