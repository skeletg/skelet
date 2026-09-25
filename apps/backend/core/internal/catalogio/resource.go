package catalogio

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

// Column names - attribute columns are the attrPrefix followed by the attribute slug, e.g. "attr:color"
const (
	colSKU            = "sku"
	colKind           = "kind"
	colParentSKU      = "parent_sku"
	colName           = "name"
	colSlug           = "slug"
	colProductType    = "product_type"
	colPrice          = "price"
	colCurrency       = "currency"
	colCategory       = "category"
	colPublished      = "published"
	colVariantMode    = "variant_mode"
	colOptions        = "options"
	colGroupedMembers = "grouped_members"
	colWeightGrams    = "weight_grams"
	colLengthMm       = "length_mm"
	colWidthMm        = "width_mm"
	colHeightMm       = "height_mm"
	attrPrefix        = "attr:"

	optCreateMissingValues = "create_missing_values"
)

var fields = []dataio.Field{
	{Name: colSKU, Label: "SKU", Type: dataio.FieldText, Identifier: true, Required: true, Description: "The upsert key - a row whose SKU already exists updates it", Example: "TEE-BLUE-M"},
	{Name: colKind, Label: "Kind", Type: dataio.FieldText, Enum: []string{"simple", "configurable", "grouped"}, Description: "Bundles and kits are built in the admin, not here", Example: "simple"},
	{Name: colParentSKU, Label: "Parent SKU", Type: dataio.FieldText, Description: "Set on a variant row to link it to its configurable parent - the parent may appear anywhere in the file", Example: "TEE"},
	{Name: colName, Label: "Name", Type: dataio.FieldText, Required: true, Description: "Written under the run's locale, leaving that locale's descriptions and SEO copy alone", Example: "Blue Tee"},
	{Name: colSlug, Label: "URL slug", Type: dataio.FieldText, Description: "Setting one switches the product to manual URLs - leave blank to keep the generated path", Example: "blue-tee"},
	{Name: colProductType, Label: "Product type", Type: dataio.FieldText, Description: "Required to create a top-level product - an existing one keeps its type and a variant inherits its parent's", Example: "clothing"},
	{Name: colPrice, Label: "Price", Type: dataio.FieldMoney, Description: "Minor units, so 1999 is 19.99", Example: "1999"},
	{Name: colCurrency, Label: "Currency", Type: dataio.FieldText, Required: true, Example: "EUR"},
	{Name: colCategory, Label: "Primary category", Type: dataio.FieldText, Description: "Category slug - additive, an existing assignment is never removed", Example: "t-shirts"},
	{Name: colPublished, Label: "Published", Type: dataio.FieldBool, Description: "Applies to the channel this run targets - ignored when no channel is selected", Example: "true"},
	{Name: colVariantMode, Label: "Variant mode", Type: dataio.FieldText, Enum: []string{"attribute", "list"}, Description: "Configurable parents only"},
	{Name: colOptions, Label: "Variant options", Type: dataio.FieldList, Description: "Attribute slugs the variants differ on, pipe-separated", Example: "color|size"},
	{Name: colGroupedMembers, Label: "Grouped members", Type: dataio.FieldList, Description: `Grouped parents only - pipe-separated "SKU" or "SKU:qty" tokens`, Example: "TEE-1|MUG-1:2"},
	{Name: colWeightGrams, Label: "Weight (g)", Type: dataio.FieldInteger, Example: "250"},
	{Name: colLengthMm, Label: "Length (mm)", Type: dataio.FieldInteger},
	{Name: colWidthMm, Label: "Width (mm)", Type: dataio.FieldInteger},
	{Name: colHeightMm, Label: "Height (mm)", Type: dataio.FieldInteger},
}

// Resource is the products entry in the data registry - one flat schema round-trips both directions, with
// configurable variants as their own rows linked by parent_sku
func Resource(queries *db.Queries, cat *catalog.Service, ch *channel.Service) dataio.Resource {
	return dataio.Resource{
		Slug:             "products",
		Label:            "Products",
		Group:            "Catalog",
		Description:      "Every product with its attributes, pricing and category. Variants are rows of their own linked to a parent by SKU.",
		ViewPermission:   "catalog.view",
		ImportPermission: "catalog.manage",
		// Product and category rows are shared by every channel that lists them
		RequireGlobalImport: true,
		Fields:              fields,
		DynamicPrefixes:     []string{attrPrefix},
		Options: []dataio.OptionDef{
			{
				Name: optCreateMissingValues, Label: "Create missing attribute values", Type: dataio.OptBool,
				Description: "A selectable attribute cell naming an unknown value creates it instead of failing the row",
				Applies:     dataio.DirectionImport,
			},
		},
		NewExporter: func(_ context.Context, o dataio.Opts) (dataio.Exporter, error) {
			return &exporter{cat: cat, ch: ch, queries: queries, opts: o}, nil
		},
		NewImporter: func(_ context.Context, o dataio.Opts) (dataio.Importer, error) {
			return &importer{
				cat:      cat,
				ch:       ch,
				opts:     o,
				rs:       newResolver(cat, ch),
				known:    map[string]bool{},
				childSeq: map[string]int32{},
			}, nil
		},
	}
}
