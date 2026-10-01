// Package catalog owns product types, attributes and the assignment between them - the structural layer products are built on, where Metadata is readable by any caller with read access and privateMetadata is operator-only (ERP ids, integration handles, fraud flags)
package catalog

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type InputType string

const (
	InputTypePlainText   InputType = "plain_text"
	InputTypeNumeric     InputType = "numeric"
	InputTypeBoolean     InputType = "boolean"
	InputTypeDate        InputType = "date"
	InputTypeDateTime    InputType = "date_time"
	InputTypeDropdown    InputType = "dropdown"
	InputTypeMultiselect InputType = "multiselect"
	InputTypeSwatch      InputType = "swatch"
	InputTypeReference   InputType = "reference" // points at another catalog entity - values live in catalog_product_attribute_references, target type declared on Attribute.RelationshipTarget
)

// Reference attributes take neither predefined values nor a scalar - their values are entity pointers
func (t InputType) AcceptsValues() bool {
	switch t {
	case InputTypeDropdown, InputTypeMultiselect, InputTypeSwatch:
		return true
	}
	return false
}

func (t InputType) IsReference() bool { return t == InputTypeReference }

func (t InputType) IsValid() bool {
	switch t {
	case InputTypePlainText, InputTypeNumeric, InputTypeBoolean,
		InputTypeDate, InputTypeDateTime,
		InputTypeDropdown, InputTypeMultiselect, InputTypeSwatch,
		InputTypeReference:
		return true
	}
	return false
}

type ReferenceTarget string // scoped to the entities catalog resolves on its own - cms_page / blog_post would need the CMS and blog services

const (
	ReferenceTargetProduct    ReferenceTarget = "product"
	ReferenceTargetCategory   ReferenceTarget = "category"
	ReferenceTargetCollection ReferenceTarget = "collection"
)

func (t ReferenceTarget) IsValid() bool {
	switch t {
	case ReferenceTargetProduct, ReferenceTargetCategory, ReferenceTargetCollection:
		return true
	}
	return false
}

type Metadata map[string]string // callers treat nil and empty as equivalent - the repository round-trips both to '{}'

// Declares what a product of this kind looks like and which attributes apply - any product can act as a configurable by having children linked to it
type ProductType struct {
	ID                 uuid.UUID
	Slug               string
	Name               string
	IsDigital          bool
	IsShippingRequired bool
	URLTemplate        string // empty = no auto-generation default - token syntax lives in url_template.go, validated against the type's assigned attributes when set
	TracksInventory    bool
	Metadata           Metadata
	PrivateMetadata    Metadata
	Attributes         []ProductTypeAttribute
	Groups             []AttributeGroup
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type AttributeGroup struct {
	ID            uuid.UUID
	ProductTypeID uuid.UUID
	Name          string
	SortOrder     int32
	Attributes    []ProductTypeAttribute
	CreatedAt     time.Time
}

type ProductTypeAttribute struct {
	Attribute Attribute
	SortOrder int32
	GroupID   *uuid.UUID // nil = ungrouped
}

type H1Position string // where an indexable filter value lands in the listing-page H1 - TEXT with a CHECK in Postgres, typed here so services pass it through without re-validating

const (
	H1PositionBefore H1Position = "before"
	H1PositionAfter  H1Position = "after"
)

func (p H1Position) IsValid() bool {
	return p == H1PositionBefore || p == H1PositionAfter
}

// Sparse - a missing locale renders the global Attribute.Name, and an empty Name means "not translated" so readers fall back even when the row exists for the description
type AttributeContent struct {
	AttributeID uuid.UUID
	Locale      string
	Name        string
	Description string
	UpdatedAt   time.Time
}

type AttributeValueContent struct {
	ValueID     uuid.UUID
	Locale      string
	Name        string
	Description string
	UpdatedAt   time.Time
}

type UpsertAttributeContentInput struct { // empty Name and Description both mean "not translated in this locale" - when both are empty the row is deleted
	AttributeID uuid.UUID
	Locale      string
	Name        string
	Description string
}

type UpsertAttributeValueContentInput struct {
	ValueID     uuid.UUID
	Locale      string
	Name        string
	Description string
}

type CascadeAttributeIndexableResult struct { // counts come back so the admin UI can confirm the cascade happened even when most flags were already in the target state
	Attribute             Attribute
	AffectedValuesCount   int64
	AffectedProductsCount int64
}

type Attribute struct {
	ID                 uuid.UUID
	Slug               string
	Name               string
	InputType          InputType
	Unit               string
	ValueRequired      bool
	Filterable         bool
	Searchable         bool
	IsPublic           bool
	FacetCollapsed     bool
	IsIndexable        bool // master SEO opt-in - a facet bucket is indexable only when this AND the per-value flag are true, so one switch disables a whole group
	H1Position         H1Position
	SynapseTrack       bool // marks the attribute commercially significant - filtering by its values or viewing products carrying them records a Synapse profile affinity
	AutoSegment        bool // a scheduled sync materialises one customer segment per popular value - only meaningful alongside SynapseTrack
	SortOrder          int32
	Metadata           Metadata
	PrivateMetadata    Metadata
	DefaultValue       json.RawMessage // storage shape mirrors the input type - scalar JSON, an AttributeValue id for dropdown/swatch, an array of ids for multiselect
	RelationshipTarget ReferenceTarget
	Values             []AttributeValue
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Per-product URL slug override for one scalar attribute (selectable slugs live on AttributeValue instead) - sparse: a missing row means slugify(value), a row exists only where the operator wants the slug decoupled (long names, SEO permalinks, rebrands that shouldn't break links), so two products sharing a brand value can carry different slugs and the storefront buckets by stored slug at index time
type ProductAttributeSlug struct {
	ID          uuid.UUID
	ProductID   uuid.UUID
	AttributeID uuid.UUID
	ChannelID   *uuid.UUID // nil = global override, set = a channel-only override that wins over the global row on that channel
	Slug        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UpsertProductAttributeSlugInput struct { // keyed by (product, attribute, channel) - re-saving overwrites, a nil ChannelID targets the global row
	ProductID   uuid.UUID
	AttributeID uuid.UUID
	ChannelID   *uuid.UUID
	Slug        string
}

type ProductAttributeLocaleValue struct { // overrides the canonical default-locale value in the product's attributes JSONB for one locale - missing row falls back to it
	ProductID   uuid.UUID
	AttributeID uuid.UUID
	Locale      string
	Value       string
	UpdatedAt   time.Time
}

type AttributeValueChannelSlug struct {
	ID          uuid.UUID
	ValueID     uuid.UUID
	AttributeID uuid.UUID
	ChannelID   uuid.UUID
	Slug        string
	UpdatedAt   time.Time
}

type AttributeValue struct { // for swatch, Value holds a hex color or an image URL - free text for the other value-carrying types
	ID          uuid.UUID
	AttributeID uuid.UUID
	Slug        string
	Name        string
	Value       string
	SortOrder   int32
	IsIndexable bool
	CreatedAt   time.Time
}

type CreateProductTypeInput struct {
	Slug               string
	Name               string
	IsDigital          bool
	IsShippingRequired bool
	URLTemplate        string // "" = no auto-generation default
	TracksInventory    bool
	Metadata           Metadata
	PrivateMetadata    Metadata
}

type UpdateProductTypeInput struct { // slug is immutable - changing it would invalidate external references that point at product types by slug
	ID                 uuid.UUID
	Name               *string
	IsDigital          *bool
	IsShippingRequired *bool
	URLTemplate        *string   // nil = no change; pointer to "" = clear
	TracksInventory    *bool     // nil = no change
	Metadata           *Metadata // nil = no change, non-nil (even empty) = replace
	PrivateMetadata    *Metadata
}

type CreateAttributeInput struct {
	Slug               string
	Name               string
	InputType          InputType
	Unit               string
	ValueRequired      bool
	Filterable         bool
	Searchable         bool
	IsPublic           bool
	FacetCollapsed     bool
	IsIndexable        bool
	H1Position         H1Position
	SynapseTrack       bool
	AutoSegment        bool
	SortOrder          int32
	Values             []AttributeValueInput // optional initial values
	Metadata           Metadata
	PrivateMetadata    Metadata
	DefaultValue       json.RawMessage // selectable defaults reference an AttributeValue id, so create the attribute with its values first and set the default via UpdateAttribute
	RelationshipTarget ReferenceTarget
}

type UpdateAttributeInput struct { // slug and InputType are left out deliberately - changing InputType would orphan attribute_values rows
	ID              uuid.UUID
	Name            *string
	Unit            *string
	ValueRequired   *bool
	Filterable      *bool
	Searchable      *bool
	IsPublic        *bool
	FacetCollapsed  *bool
	IsIndexable     *bool
	H1Position      *H1Position
	SynapseTrack    *bool
	AutoSegment     *bool
	SortOrder       *int32
	Metadata        *Metadata
	PrivateMetadata *Metadata
	DefaultValue    *json.RawMessage // a pointer wrapping a nil RawMessage clears, a non-nil RawMessage sets, nil is no change
}

type AttributeValueInput struct {
	Slug        string
	Name        string
	Value       string
	SortOrder   int32
	IsIndexable bool
}

type AssignAttributeInput struct {
	ProductTypeID uuid.UUID
	AttributeID   uuid.UUID
	SortOrder     int32
	GroupID       *uuid.UUID
}

type CreateAttributeGroupInput struct {
	ProductTypeID uuid.UUID
	Name          string
	SortOrder     int32
}

type UpdateAttributeGroupInput struct {
	ID        uuid.UUID
	Name      *string
	SortOrder *int32
}

var (
	ErrProductTypeNotFound               = errors.New("product type not found")
	ErrAttributeGroupNotFound            = errors.New("attribute group not found")
	ErrAttributeNotFound                 = errors.New("attribute not found")
	ErrAttributeValueNotFound            = errors.New("attribute value not found")
	ErrProductAttributeSlugNotFound      = errors.New("product attribute slug not found")
	ErrSlugTaken                         = errors.New("slug already in use")
	ErrInvalidSlug                       = errors.New("invalid slug")
	ErrInvalidInputType                  = errors.New("invalid attribute input type")
	ErrValuesNotApplicable               = errors.New("input type does not accept values")
	ErrAssignmentNotFound                = errors.New("attribute is not assigned to product type")
	ErrAttributeUsedAsConfigurableOption = errors.New("attribute is used as a configurable option on one or more products")
	ErrAttributeValueIsDefault           = errors.New("attribute value is referenced as the attribute's default - clear the default first")
	ErrInvalidDefaultValue               = errors.New("invalid attribute default value")
	ErrRequiredAttributeNeedsDefault     = errors.New("required attributes must have a default value") // required implies default_value IS NOT NULL - without it CreateProduct's defaults-fill would silently let products exist with no value, making "required" meaningless
	ErrProductContentNotFound            = errors.New("product content not found for this locale")
	ErrLocaleMustBeDefault               = errors.New("initial content must use the default locale")                   // every entity is born in the canonical locale - translations arrive later via UpsertContent
	ErrCannotDeleteDefaultLocale         = errors.New("cannot delete default-locale content while translations exist") // removing the default would leave translations with nothing to fall back to
	ErrDuplicateFacetOverride            = errors.New("duplicate attribute in facet overrides")
	ErrNameRequired                      = errors.New("name is required")
	ErrLocaleRequired                    = errors.New("locale is required")
	ErrInvalidSize                       = errors.New("size_bytes must be positive")
	ErrInvalidDimensions                 = errors.New("width and height must be positive")
	ErrTooManyItems                      = errors.New("too many items in one request")
	ErrInvalidH1Position                 = errors.New("invalid h1Position")
	ErrAttributeGroupNameTaken           = errors.New("an attribute group with this name already exists on the product type")
	ErrAttributeGroupWrongType           = errors.New("attribute group belongs to a different product type")
	ErrSelectableSlugOverride            = errors.New("selectable attributes manage slugs through AttributeValue, not per-product overrides")
	ErrAttributeNotTranslatable          = errors.New("only plain_text scalar attribute values are translatable")
	ErrMediaStorageUnavailable           = errors.New("media storage not configured")
	ErrInvalidLinkKind                   = errors.New("invalid product link kind")
	ErrInvalidTierQuantity               = errors.New("tier min quantity must be greater than zero")
	ErrCustomerGroupNotFound             = errors.New("customer group not found")
)

// No producer-defined repository interfaces here - a consumer that needs to mock a slice of persistence declares the narrow interface it uses, in its own package
