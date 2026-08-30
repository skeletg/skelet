package model

// Hand-rolled inverse relationships behind Product.partOf, pinned via the `models:` block in gqlgen.admin.yml so gqlgen generates dedicated structs instead of resolvers

type ProductMemberships struct {
	ProductID string `json:"-"` // carried so the sub-resolvers know which product to look up, never serialised
}

type ConfigurableMembership struct {
	Parent       *Product                 `json:"parent"`
	VariantMode  *VariantMode             `json:"variantMode,omitempty"`
	OptionValues []*ProductAttributeValue `json:"optionValues"`
	SortOrder    int32                    `json:"sortOrder"`
	Siblings     []*Product               `json:"siblings"`
}

type BundleMembership struct {
	Bundle    *Product           `json:"bundle"`
	Option    *BundleOption      `json:"option"`
	Quantity  float64            `json:"quantity"`
	IsDefault bool               `json:"isDefault"`
	SortOrder int32              `json:"sortOrder"`
	Siblings  []*BundleSelection `json:"siblings"`
}

type GroupedMembership struct {
	GroupedProduct  *Product         `json:"groupedProduct"`
	DefaultQuantity float64          `json:"defaultQuantity"`
	SortOrder       int32            `json:"sortOrder"`
	Siblings        []*GroupedMember `json:"siblings"`
}
