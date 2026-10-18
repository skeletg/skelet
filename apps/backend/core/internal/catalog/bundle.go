package catalog

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type BundleInputType string // menu UI affordance - the pairs are a visual hint, the backend only distinguishes single from multi

const (
	BundleInputRadio       BundleInputType = "radio"
	BundleInputDropdown    BundleInputType = "dropdown"
	BundleInputCheckbox    BundleInputType = "checkbox"
	BundleInputMultiselect BundleInputType = "multiselect"
)

func (t BundleInputType) IsKnown() bool {
	switch t {
	case BundleInputRadio, BundleInputDropdown, BundleInputCheckbox, BundleInputMultiselect:
		return true
	}
	return false
}

func (t BundleInputType) IsSingleSelect() bool {
	return t == BundleInputRadio || t == BundleInputDropdown
}

func (t BundleInputType) IsMultiSelect() bool {
	return t == BundleInputCheckbox || t == BundleInputMultiselect
}

type BundleOption struct { // one "option group" on a bundle product - a single "Pick a thing" slot
	ID            uuid.UUID
	ProductID     uuid.UUID
	InputType     BundleInputType
	IsRequired    bool
	SortOrder     int32
	MinSelections int32  // 0 = no explicit minimum, though an IsRequired option still demands >=1 - single-select ignores it
	MaxSelections *int32 // nil = unbounded - single-select ignores it
	Title         string // resolved to the active request locale with a default-locale fallback - empty when no content row exists
	AllContent    []BundleOptionContent
	Selections    []BundleSelection
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type BundleOptionContent struct {
	Locale string
	Title  string
}

type BundleSelection struct { // one SIMPLE product filling an option's menu
	ID                  uuid.UUID
	OptionID            uuid.UUID
	ProductID           uuid.UUID
	Quantity            float64 // per ONE unit of the bundle, multiplied by the cart-line quantity at add-to-cart time
	IsDefault           bool
	SortOrder           int32
	ProductURL          string // display fields from here down are populated by GetBundleOptions - zero when only the raw row is loaded
	ProductSKU          string
	ProductName         string
	ProductDefaultPrice int64
	ProductCurrency     string
}

type AddBundleOptionInput struct {
	ProductID     uuid.UUID
	InputType     BundleInputType
	IsRequired    bool
	SortOrder     int32
	MinSelections int32
	MaxSelections *int32
	Locale        string // writes the first content row alongside the option - must equal the configured default locale, matching the rule products use
	Title         string
}

type UpdateBundleOptionInput struct {
	ID                 uuid.UUID
	InputType          *BundleInputType
	IsRequired         *bool
	SortOrder          *int32
	MinSelections      *int32 // nil = no change, here and below - ClearMaxSelections sets MaxSelections back to NULL (unbounded) and is mutually exclusive with a non-nil MaxSelections
	MaxSelections      *int32
	ClearMaxSelections bool
}

type UpsertBundleOptionContentInput struct {
	OptionID uuid.UUID
	Locale   string
	Title    string
}

type AddBundleSelectionInput struct {
	OptionID  uuid.UUID
	ProductID uuid.UUID
	Quantity  float64
	IsDefault bool
	SortOrder int32
}

type UpdateBundleSelectionInput struct {
	ID        uuid.UUID
	Quantity  *float64
	IsDefault *bool
	SortOrder *int32
}

var (
	ErrBundleParentNotBundle      = errors.New("bundle options can only be defined on products of kind BUNDLE")
	ErrBundleSelectionNotSimple   = errors.New("bundle selections must reference SIMPLE products")
	ErrBundleSelectionIsParent    = errors.New("a bundle cannot include itself as a selection")
	ErrBundleOptionNotFound       = errors.New("bundle option not found")
	ErrBundleSelectionNotFound    = errors.New("bundle selection not found")
	ErrBundleSelectionMissing     = errors.New("bundle requires a selection for a required option")
	ErrBundleSelectionInvalid     = errors.New("bundle selection does not belong to this bundle")
	ErrBundleSelectionCardinality = errors.New("bundle option does not allow this many selections")
	ErrBundleMultiDefaultOnSingle = errors.New("single-select bundle options accept at most one default selection")
	ErrInvalidBundleInputType     = errors.New("invalid bundle input type")
	ErrInvalidBundleLimits        = errors.New("invalid bundle option selection limits")
	ErrBundleSelectionDuplicate   = errors.New("this product is already a selection on the option")
	ErrBundleDefaultsExceedMax    = errors.New("bundle option has more default selections than max_selections allows")
)
