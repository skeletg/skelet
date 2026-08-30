package model

import (
	adminmodel "github.com/skeletg/skelet/graph/admin/model"
)

// Product.partOf inverse relationships, pinned in gqlgen.front.yml's `models:` block - the ProductMemberships field resolvers fill these in whole, so every leaf below binds straight to a struct field with no per-leaf resolver

type ProductMemberships struct {
	ProductID string `json:"-"` // lazy shell - nothing is queried until configurable / bundles / groupedProducts is selected
}

type ConfigurableMembership struct {
	Parent       *adminmodel.Product                 `json:"parent"`
	VariantMode  *adminmodel.VariantMode             `json:"variantMode,omitempty"`
	OptionValues []*adminmodel.ProductAttributeValue `json:"optionValues"`
	SortOrder    int32                               `json:"sortOrder"`
	Siblings     []*adminmodel.Product               `json:"siblings"`
}

type BundleMembership struct {
	Bundle    *adminmodel.Product           `json:"bundle"`
	Option    *adminmodel.BundleOption      `json:"option"`
	Quantity  float64                       `json:"quantity"`
	IsDefault bool                          `json:"isDefault"`
	SortOrder int32                         `json:"sortOrder"`
	Siblings  []*adminmodel.BundleSelection `json:"siblings"`
}

type GroupedMembership struct {
	GroupedProduct  *adminmodel.Product         `json:"groupedProduct"`
	DefaultQuantity float64                     `json:"defaultQuantity"`
	SortOrder       int32                       `json:"sortOrder"`
	Siblings        []*adminmodel.GroupedMember `json:"siblings"`
}
