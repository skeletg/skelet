package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type ProductContent struct {
	Locale           string
	Name             string
	ShortDescription json.RawMessage // Lexical rich-text JSON; nil = no short description
	Description      json.RawMessage // Lexical rich-text JSON; nil = no description set
	MetaTitle        *string
	MetaDescription  *string
	MetaKeywords     *string
	UpdatedAt        time.Time
}

// ProductKind is the structural shape - not ProductType, which is the attribute schema - Immutable after create
type ProductKind string

const (
	KindSimple       ProductKind = "simple"
	KindConfigurable ProductKind = "configurable"
	KindBundle       ProductKind = "bundle"
	KindGrouped      ProductKind = "grouped"
	KindKit          ProductKind = "kit"
	KindGiftcard     ProductKind = "giftcard"
	KindSubscription ProductKind = "subscription"
)

func (k ProductKind) IsKnown() bool {
	switch k {
	case KindSimple, KindConfigurable, KindBundle, KindGrouped, KindKit, KindGiftcard, KindSubscription:
		return true
	}
	return false
}

func (k ProductKind) IsPurchasable() bool {
	return k == KindSimple || k == KindConfigurable || k == KindBundle ||
		k == KindKit || k == KindGiftcard || k == KindSubscription
}

// IsCartLineEligible is the narrower "can be a direct flat AddToCart line priced off its own SKU" check, vs IsPurchasable's broad one
func (k ProductKind) IsCartLineEligible() bool {
	return k == KindSimple || k == KindKit || k == KindGiftcard || k == KindSubscription
}

func (k ProductKind) OwnsInventory() bool {
	return k == KindSimple
}

// VariantMode only means anything on a configurable, where nil is treated as VariantModeAttribute (children differ by option-value combo) rather than a curated list
type VariantMode string

const (
	VariantModeAttribute VariantMode = "attribute"
	VariantModeList      VariantMode = "list"
)

func (m VariantMode) IsValid() bool {
	return m == VariantModeAttribute || m == VariantModeList
}

// Scalar attribute values live in the Attributes JSONB blob - selectable ones are relational, loaded into AttributeSelections
type Product struct {
	ID            uuid.UUID
	ProductTypeID uuid.UUID
	Kind          ProductKind
	VariantMode   *VariantMode
	// Inheritance source for listings with a NULL price, and only when the store currency matches - customers always read a listing row
	DefaultPrice        int64
	DefaultCurrencyCode string
	CompareAtPrice      *int64
	// Active within [SpecialFrom, SpecialTo), open-ended on a nil bound - channel listings override the trio, resolution lives in internal/pricing
	SpecialPrice *int64
	SpecialFrom  *time.Time
	SpecialTo    *time.Time
	WeightGrams  *int32
	LengthMm     *int32
	WidthMm      *int32
	HeightMm     *int32
	MinOrderQty  *float64
	MaxOrderQty  *float64
	CostCents    *int64
	// Required on wrappers and variants alike - "configurable" is derived from child rows, not from the SKU
	SKU          string
	TaxClass     string
	SEONoindex   bool
	CanonicalURL *string
	// Single canonical path - url_rewrites enforces uniqueness of the resolved path (suffix -1, -2 on collision), per-store overrides live on the listing
	URL                     string
	URLAuto                 bool
	URLTemplateOverride     string
	TracksInventoryOverride *bool
	QuantityDecimals        int16
	// Smallest sellable increment (0.25 for quarter meters) - inputs snap to multiples and the service rejects writes that don't
	QuantityStep float64
	// Everything from Content down is hydrated by GetProductWithAttributes, not by a plain read
	Content             *ProductContent
	AllContent          []ProductContent
	Attributes          ProductScalarAttributes // scalar attribute values keyed by attribute.slug
	Metadata            Metadata
	PrivateMetadata     Metadata
	AttributeSelections []ProductAttributeSelection
	// Label/Path are unresolved here - the GraphQL layer hydrates them per channel/locale
	AttributeReferences []ProductAttributeReference
	Children            []Product
	ConfigurableOptions []Attribute
	// Collections and Categories are the exception - the resolver fills them via ListCollectionsByProductIDs / ListCategoriesByProductIDs
	Collections     []Collection
	Categories      []Category
	PrimaryCategory *Category
	MediaRoles      map[MediaRole]uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ProductScalarAttributes map[string]any

type ProductAttributeSelection struct {
	AttributeID uuid.UUID
	Value       AttributeValue
	SortOrder   int32
}

type ProductAttributeReference struct {
	AttributeID uuid.UUID
	TargetType  ReferenceTarget
	TargetID    uuid.UUID
	Position    int32
	Label       string
	Path        string
}

type CreateProductInput struct {
	ProductTypeID uuid.UUID
	// Required - bundle and grouped are rejected at create time until those features land
	Kind                ProductKind
	DefaultPrice        int64
	DefaultCurrencyCode string
	// Empty falls back to a slugified name
	SKU                     string
	SEONoindex              bool
	CanonicalURL            string
	URL                     *string // nil / "" → auto-generate (template, else slugified name)
	URLAuto                 *bool
	URLTemplateOverride     string // "" = inherit product type's template
	TracksInventoryOverride *bool
	QuantityDecimals        int16
	// Defaults to 1 / 10^QuantityDecimals when omitted
	QuantityStep     float64
	Metadata         Metadata
	PrivateMetadata  Metadata
	Locale           string
	Name             string
	ShortDescription json.RawMessage // Lexical rich-text JSON; nil = no short description
	Description      json.RawMessage // Lexical rich-text JSON; nil = no description
	MetaTitle        string
	MetaDescription  string
	MetaKeywords     string
}

// GenerateConfigurableChildInput materialises one SIMPLE product per variant, then links it as a child of the parent
type GenerateConfigurableChildInput struct {
	// Exactly one value per configurable option on the parent, any order
	AttributeValueIDs   []uuid.UUID
	Name                string
	SKU                 string
	DefaultPrice        int64
	DefaultCurrencyCode string
	Locale              string
	SortOrder           int32
}

// Nil pointers mean "no change" and ProductTypeID is immutable - name/description/seo copy goes through UpsertProductContent instead
type UpdateProductInput struct {
	ID                  uuid.UUID
	DefaultPrice        *int64
	DefaultCurrencyCode *string
	// Mandatory at all times, so empty is rejected - there is no "clear SKU"
	SKU          *string
	SEONoindex   *bool
	CanonicalURL *string // nil = no change; pointer to empty string = clear
	URL          *string // nil = no change; pointer to empty string = clear (reverts to auto-generate)
	// Setting true while URL is also set is rejected (ErrManualPathWhileAuto)
	URLAuto                      *bool
	URLTemplateOverride          *string // nil = no change
	TracksInventoryOverride      *bool
	ClearTracksInventoryOverride bool
	// When decimals change and step is nil the service re-derives 1 / 10^decimals
	QuantityDecimals *int16
	QuantityStep     *float64
	Metadata         *Metadata
	PrivateMetadata  *Metadata
	// ClearSpecialPrice clears the whole price/from/to trio
	CompareAtPrice      *int64
	ClearCompareAtPrice bool
	SpecialPrice        *int64
	SpecialFrom         *time.Time
	SpecialTo           *time.Time
	ClearSpecialPrice   bool
	// Pointer-to-zero clears - 0 is stored as NULL, "unspecified" rather than "weightless"
	WeightGrams    *int32
	LengthMm       *int32
	WidthMm        *int32
	HeightMm       *int32
	MinOrderQty    *float64
	MaxOrderQty    *float64
	CostCents      *int64 // 0 is a real cost (a free sample) - clearing is ClearCostCents
	ClearCostCents bool
}

type Dimensions struct {
	WeightGrams, LengthMm, WidthMm, HeightMm int32
	ProductTypeID                            uuid.UUID
}

type ProductContentInput struct {
	Locale           string
	Name             string
	ShortDescription json.RawMessage // Lexical rich-text JSON; nil = clear short description
	Description      json.RawMessage // Lexical rich-text JSON; nil = clear description
	MetaTitle        string
	MetaDescription  string
	MetaKeywords     string
}

// Declared here so catalog -> urlrewrite stays a one-way dependency and tests can stub it
type UrlRewriteSyncer interface {
	SyncCanonical(ctx context.Context, entityType string, entityID uuid.UUID, requestPath string) error
	CreateRedirect(ctx context.Context, from string, entityType string, entityID uuid.UUID, redirectType int) error
	DeleteAllForEntity(ctx context.Context, entityType string, entityID uuid.UUID) error
}

// The target attribute's input_type picks the storage path - selectable uses ValueIDs, scalar uses ScalarValue
// Clear with an empty ValueIDs or a nil ScalarValue; omitting the attribute leaves it unchanged
type AttributeAssignment struct {
	AttributeID uuid.UUID
	ValueIDs    []uuid.UUID // for selectable attributes
	ScalarValue any         // for scalar attributes (nil = clear)
	HasScalar   bool        // distinguishes "no value provided" from "value is nil/empty"
}

var (
	ErrProductNotFound                    = errors.New("product not found")
	ErrAttributeNotAssignedToType         = errors.New("attribute is not assigned to this product type")
	ErrScalarValueOnSelectableAttr        = errors.New("selectable attribute requires valueIds, not scalarValue")
	ErrSelectableValueOnScalarAttr        = errors.New("scalar attribute requires scalarValue, not valueIds")
	ErrAttributeValueMismatch             = errors.New("attribute value does not belong to this attribute")
	ErrInvalidScalarType                  = errors.New("scalar value does not match attribute input type")
	ErrDropdownRequiresSingleValue        = errors.New("dropdown/swatch attributes require exactly one value")
	ErrRequiredAttributeMissing           = errors.New("required attribute missing")
	ErrInvalidCurrency                    = errors.New("currency_code must be a 3-letter ISO 4217 code")
	ErrProductChildNotFound               = errors.New("product child not found under this parent")
	ErrAlreadyLinked                      = errors.New("this child is already linked to this parent")
	ErrChildAlreadyHasParent              = errors.New("this child is already a variant of another configurable; a variant may belong to only one configurable")
	ErrCannotAddSelfAsChild               = errors.New("a product cannot be its own child")
	ErrCyclicChildLink                    = errors.New("adding this child would create a circular parent-child chain")
	ErrChildProductTypeMismatch           = errors.New("child product must have the same product type as the parent")
	ErrDuplicateSuperAttributeCombination = errors.New("a child with the same super attribute combination already exists")
	ErrConfigurableOptionNotSelectable    = errors.New("configurable option attributes must be selectable (dropdown, multiselect, or swatch)")
	ErrConfigurableOptionNotAssigned      = errors.New("configurable option attribute is not assigned to this product type")
	ErrChildMissingConfigurableOptions    = errors.New("child product is missing values for one or more configurable option attributes")
	ErrSKUTaken                           = errors.New("sku already in use")
	ErrInvalidReferenceTarget             = errors.New("relationshipTarget is invalid or misused for this input type")
	ErrAttributeNotReference              = errors.New("attribute is not a reference type")
	ErrReferenceTargetMismatch            = errors.New("reference target type does not match the attribute's relationshipTarget")
	ErrReferenceTargetNotFound            = errors.New("reference target entity does not exist")
	ErrInvalidProductKind                 = errors.New("invalid product kind")
	ErrParentNotConfigurable              = errors.New("parent product must be of kind 'configurable' to accept children")
	ErrChildNotSimple                     = errors.New("only 'simple' products can be linked as children of a configurable")
	ErrConfigurableNeedsOption            = errors.New("attribute-mode configurable requires at least one configurable option before children can be added")
	ErrListModeForbidsOptions             = errors.New("list-mode configurable cannot have configurable options")
	ErrTemplateEmpty                      = errors.New("url path template is empty")
	ErrTemplateMissingSlugToken           = errors.New("url path template must include {slug}")
	ErrTemplateUnknownToken               = errors.New("unknown url path template token")
	ErrTemplateUnbalancedBraces           = errors.New("url path template has unbalanced braces")
	ErrTemplateAttributeNotAssigned       = errors.New("url path template references an attribute that is not assigned to the product type")
	ErrTemplateAttributeMultiselect       = errors.New("url path template cannot reference a multiselect attribute")
	ErrTemplateMissingCategory            = errors.New("url path template requires a primary category")
	ErrTemplateMissingAttribute           = errors.New("url path template requires an attribute value that is not set on the product")
	ErrManualPathWhileAuto                = errors.New("urlPath cannot be set manually while urlPathAuto is true")
	ErrNegativePrice                      = errors.New("prices must be non-negative")
	ErrSKURequired                        = errors.New("sku is required and cannot be cleared")
	ErrSKUInActiveCarts                   = errors.New("sku is in active carts; it cannot be renamed until they check out or expire")
	ErrConfigurableOptionsLocked          = errors.New("cannot change configurable options while children are linked; remove all children first")
	ErrDuplicateConfigurableOption        = errors.New("duplicate attribute in configurable options")
	ErrInvalidVariantMode                 = errors.New("invalid variant mode")
	ErrNoVariants                         = errors.New("at least one variant is required")
	ErrInvalidVariantSelection            = errors.New("variant option values do not match the parent's configurable options")
)
