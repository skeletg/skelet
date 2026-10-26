package catalog

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Customer-facing UI shape of an option - text (field, area), select (dropdown, radio, checkbox, multiselect), date (date, date_time, time) and file
// Only select-family types have value rows; the rest carry any price adjustment on the option itself
type ProductOptionInputType string

const (
	OptionInputField       ProductOptionInputType = "field"
	OptionInputArea        ProductOptionInputType = "area"
	OptionInputDropdown    ProductOptionInputType = "dropdown"
	OptionInputRadio       ProductOptionInputType = "radio"
	OptionInputCheckbox    ProductOptionInputType = "checkbox"
	OptionInputMultiselect ProductOptionInputType = "multiselect"
	OptionInputDate        ProductOptionInputType = "date"
	OptionInputDateTime    ProductOptionInputType = "date_time"
	OptionInputTime        ProductOptionInputType = "time"
	OptionInputFile        ProductOptionInputType = "file"
)

func (t ProductOptionInputType) IsKnown() bool {
	switch t {
	case OptionInputField, OptionInputArea,
		OptionInputDropdown, OptionInputRadio, OptionInputCheckbox, OptionInputMultiselect,
		OptionInputDate, OptionInputDateTime, OptionInputTime,
		OptionInputFile:
		return true
	}
	return false
}

func (t ProductOptionInputType) IsSelect() bool {
	switch t {
	case OptionInputDropdown, OptionInputRadio, OptionInputCheckbox, OptionInputMultiselect:
		return true
	}
	return false
}

func (t ProductOptionInputType) IsSingleSelect() bool {
	return t == OptionInputDropdown || t == OptionInputRadio
}

func (t ProductOptionInputType) IsMultiSelect() bool {
	return t == OptionInputCheckbox || t == OptionInputMultiselect
}

func (t ProductOptionInputType) IsText() bool {
	return t == OptionInputField || t == OptionInputArea
}

func (t ProductOptionInputType) IsDate() bool {
	return t == OptionInputDate || t == OptionInputDateTime || t == OptionInputTime
}

func (t ProductOptionInputType) IsFile() bool {
	return t == OptionInputFile
}

// FIXED stores cents, PERCENT stores integer percent points (10 = 10% of the product's resolved channel unit price)
type ProductOptionPriceType string

const (
	OptionPriceFixed   ProductOptionPriceType = "fixed"
	OptionPricePercent ProductOptionPriceType = "percent"
)

func (t ProductOptionPriceType) IsKnown() bool {
	return t == OptionPriceFixed || t == OptionPricePercent
}

// One visibility condition on a PRIOR option of the same product - anyOf/noneOf match a select-family parent's picked ValueIDs, filled/empty work on any parent
type OptionRule struct {
	OptionID uuid.UUID   `json:"option_id"`
	Op       string      `json:"op"`
	ValueIDs []uuid.UUID `json:"value_ids,omitempty"`
}

// AND/OR tree of rules - empty logic means AND, a nil group means always visible
type OptionConditionGroup struct {
	Logic  string                 `json:"logic,omitempty"`
	Rules  []OptionRule           `json:"rules,omitempty"`
	Groups []OptionConditionGroup `json:"groups,omitempty"`
}

const (
	OptionRuleAnyOf  = "anyOf"
	OptionRuleNoneOf = "noneOf"
	OptionRuleFilled = "filled"
	OptionRuleEmpty  = "empty"
	OptionLogicAnd   = "and"
	OptionLogicOr    = "or"
)

var OptionRuleOps = map[string]bool{OptionRuleAnyOf: true, OptionRuleNoneOf: true, OptionRuleFilled: true, OptionRuleEmpty: true}

type ProductOption struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	// Set instead of ProductID on a template's option - TemplateName is resolved only in a product's list
	TemplateID   uuid.UUID
	TemplateName string
	InputType    ProductOptionInputType
	IsRequired   bool
	SortOrder    int32
	// Title and HelpText are resolved to the active request locale with a default-locale fallback
	Title      string
	HelpText   string
	AllContent []ProductOptionContent
	// Always nil for select-family options - their per-value rows carry the adjustments instead
	PriceValue     *int32
	PriceType      *ProductOptionPriceType
	MaxChars       *int32
	MaxFileBytes   *int64
	FileExtensions string
	Values         []ProductOptionValue
	// Gates the option on choices made for EARLIER options (strictly lower sort_order) - nil = always visible
	VisibleIf *OptionConditionGroup
	// A media-library picture shown beside the option - nil for none
	ImageAssetID *uuid.UUID
	// The owning product's default currency, which a bare FIXED amount is in - resolved only by GetProductOptions, like ChannelPrices on the option and its values
	Currency      string
	ChannelPrices []OptionChannelPrice
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type OptionChannelPrice struct {
	ChannelID  uuid.UUID
	PriceValue int32
}

// HelpText renders under the input on the storefront - usage hints, constraints
type ProductOptionContent struct {
	Locale   string
	Title    string
	HelpText string
}

// One pre-defined choice on a select-family option, optionally backed by an existing SIMPLE product
// When SourceProductID is set the cart contribution is that product's price plus PriceValue as a delta, and PriceType is forced to FIXED since PERCENT has no unambiguous base
// A linked value does not reserve the source product's stock - the line reserves only its own product
type ProductOptionValue struct {
	ID         uuid.UUID
	OptionID   uuid.UUID
	SortOrder  int32
	PriceValue int32
	PriceType  ProductOptionPriceType
	IsDefault  bool
	// Units the value represents - the cart multiplies its contribution by this, linked or not, always >= 1
	Quantity float64
	// Picking this value forces the cart LINE quantity ("Set of 4" -> 4) and locks it against manual edits, but only while the owning option is visible
	ForcedLineQty *float64
	// A media-library picture the storefront shows the choice as - nil for none
	ImageAssetID    *uuid.UUID
	Title           string // resolved
	AllContent      []ProductOptionValueContent
	SourceProductID *uuid.UUID
	// Display fields resolved only by GetProductOptions, and only when SourceProductID is set
	SourceProductURL          string
	SourceProductSKU          string
	SourceProductName         string
	SourceProductDefaultPrice int64
	SourceProductCurrency     string
	ChannelPrices             []OptionChannelPrice
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type ProductOptionValueContent struct {
	Locale string
	Title  string
}

type AddProductOptionInput struct {
	// Exactly one of the two
	ProductID  uuid.UUID
	TemplateID uuid.UUID
	InputType  ProductOptionInputType
	IsRequired bool
	SortOrder  int32
	// Must be nil for select-family input types - the service rejects with ErrOptionPriceOnSelect
	PriceValue     *int32
	PriceType      *ProductOptionPriceType
	MaxChars       *int32
	MaxFileBytes   *int64
	FileExtensions string
	// Initial-content row - locale must equal the configured default
	Locale       string
	Title        string
	HelpText     string
	VisibleIf    *OptionConditionGroup
	ImageAssetID *uuid.UUID
}

type UpdateProductOptionInput struct {
	ID             uuid.UUID
	InputType      *ProductOptionInputType
	IsRequired     *bool
	SortOrder      *int32
	PriceValue     *int32 // nil = leave unchanged; non-nil = set to that value
	PriceType      *ProductOptionPriceType
	MaxChars       *int32
	MaxFileBytes   *int64
	FileExtensions *string
	// nil leaves it unchanged, ClearVisibleIf drops the condition (tri-state, mirrors ClearCoords on locations)
	VisibleIf      *OptionConditionGroup
	ClearVisibleIf bool
	// Clear flags for the other nullable fields - same tri-state contract, since a GraphQL null can't be told apart from an omitted field once decoded to a pointer
	ClearPrice          bool // drops price_value + price_type together (the shape validator requires the pair)
	ClearMaxChars       bool
	ClearMaxFileBytes   bool
	ClearFileExtensions bool
	// Pointer-to-uuid sets, pointer-to-zero clears, nil is no change
	ImageAssetID *uuid.UUID
}

type UpsertProductOptionContentInput struct {
	OptionID uuid.UUID
	Locale   string
	Title    string
	HelpText string
}

type AddProductOptionValueInput struct {
	OptionID   uuid.UUID
	SortOrder  int32
	PriceValue int32
	PriceType  ProductOptionPriceType
	IsDefault  bool
	// Units the value represents, at least 1
	Quantity        float64
	SourceProductID *uuid.UUID
	ForcedLineQty   *float64
	ImageAssetID    *uuid.UUID
	Locale          string
	Title           string
}

type UpdateProductOptionValueInput struct {
	ID         uuid.UUID
	SortOrder  *int32
	PriceValue *int32
	PriceType  *ProductOptionPriceType
	IsDefault  *bool
	Quantity   *float64
	// Pointer-to-uuid sets, pointer-to-zero-uuid unlinks, nil is no change
	SourceProductID *uuid.UUID
	// Pointer sets, pointer-to-zero-or-negative clears, nil is no change
	ForcedLineQty *float64
	// Pointer-to-uuid sets, pointer-to-zero clears, nil is no change
	ImageAssetID *uuid.UUID
}

type UpsertProductOptionValueContentInput struct {
	ValueID uuid.UUID
	Locale  string
	Title   string
}

var (
	ErrProductOptionNotFound          = errors.New("product option not found")
	ErrProductOptionValueNotFound     = errors.New("product option value not found")
	ErrInvalidProductOptionInputType  = errors.New("invalid product option input type")
	ErrInvalidProductOptionPriceType  = errors.New("invalid product option price type")
	ErrOptionInputKindNotPurchasable  = errors.New("product options can only be attached to simple and configurable products - other kinds never resolve them at add-to-cart")
	ErrOptionRequiredNeedsValues      = errors.New("a required choice option needs at least one value or the product can never be purchased")
	ErrOptionValueOnNonSelect         = errors.New("only select-family options accept values (dropdown, radio, checkbox, multiselect)")
	ErrOptionPriceOnSelect            = errors.New("select-family options carry price on their values, not on the option itself")
	ErrOptionMultiDefaultOnSingle     = errors.New("single-select option (dropdown/radio) accepts at most one default value")
	ErrOptionTextConstraintsOnNonText = errors.New("max_chars is only valid for text options (field, area)")
	ErrOptionFileConstraintsOnNonFile = errors.New("max_file_bytes / file_extensions are only valid for file options")
	ErrOptionValueSourceNotSimple     = errors.New("option value source product must be of kind 'simple'")
	ErrOptionVisibilityInvalid        = errors.New("invalid option visibility condition")
	ErrOptionValuePercentOnLinked     = errors.New("PERCENT price type is not allowed on values linked to a source product")
	ErrOptionPriceIncomplete          = errors.New("price_value and price_type must be set together")
	ErrInvalidForcedLineQty           = errors.New("forced line quantity must be a whole number between 1 and the line cap")
	ErrOptionHasNoContent             = errors.New("product option has no content rows")
	ErrOptionChannelPriceNotFixed     = errors.New("only a FIXED surcharge takes a per-channel amount - a PERCENT one already follows the channel price")
	ErrOptionChannelPriceRange        = errors.New("per-channel surcharge is out of range")
	ErrOptionChannelNotListed         = errors.New("the product is not listed on this channel")
	ErrOptionImageInvalid             = errors.New("an option picture must be an uploaded media-library image")
)

// Percent adjustments outside this range are typos, not prices - below -100 the line goes negative by definition
const (
	MinOptionPercent          = -100
	MaxOptionPercent          = 1000
	MaxOptionChannelSurcharge = 1_000_000_000
)

// ParentOptionsNotOn is the configurable parent's options a variant doesn't carry already - a template on both, or on
// their shared product type, is asked once
func ParentOptionsNotOn(variant, parent []ProductOption) []ProductOption {
	have := make(map[uuid.UUID]bool, len(variant))
	for _, o := range variant {
		have[o.ID] = true
	}
	return slices.DeleteFunc(slices.Clone(parent), func(o ProductOption) bool { return have[o.ID] })
}

// ParentRulesNotOn is ParentOptionsNotOn for rules - a shared template's rule applies once, not twice
func ParentRulesNotOn(variant, parent []ProductOptionRule) []ProductOptionRule {
	have := make(map[uuid.UUID]bool, len(variant))
	for _, r := range variant {
		have[r.ID] = true
	}
	return slices.DeleteFunc(slices.Clone(parent), func(r ProductOptionRule) bool { return have[r.ID] })
}
