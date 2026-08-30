package catalog

import "github.com/google/uuid"

// Inverse-relationship links answering "what does this product belong to?" - only the parent id plus position/quantity, so they stay cheap and batchable

type ConfigurableParentLink struct {
	ParentID         uuid.UUID
	VariantSortOrder int32
}

type BundleSelectionLink struct {
	BundleID  uuid.UUID
	OptionID  uuid.UUID
	Quantity  float64
	IsDefault bool
	SortOrder int32
}

type GroupedMemberLink struct {
	ParentID        uuid.UUID
	DefaultQuantity float64
	SortOrder       int32
}
