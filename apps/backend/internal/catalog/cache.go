package catalog

import (
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/revalidate"
)

const (
	CacheProductMode     = "cache/product/mode"            // off | invalidate | rebuild
	CacheProductOnUpdate = "cache/product/on_update"       // ProductUpdated (incl. price)
	CacheProductOnStock  = "cache/product/on_stock_change" // inventory StockChanged
	CacheProductOnDelete = "cache/product/on_delete"       // ProductDeleted (always invalidate)
)

func init() {
	revalidate.RegisterSurface(revalidate.Surface{ // no AllTag on purpose - products invalidate PER-ID only, and a catalog-wide bust would mark every PDP stale and stampede the renderer on the next traffic wave
		Key:            "product",
		Label:          "Products (PDP)",
		AllTag:         "",
		DefaultEnabled: true,
	})

	revalidate.RegisterSurface(revalidate.Surface{ // unlike products this has an AllTag so an operator can bust the whole surface at once - normal invalidation stays targeted via category:<id>, collection:<id> and catalog:browse
		Key:            "catalog",
		Label:          "Catalog listings (categories, grids, collections)",
		AllTag:         "catalog:all",
		DefaultEnabled: true,
	})

	config.Register(config.Definition{
		Path:    CacheProductMode,
		Type:    config.TypeSelect,
		Default: "invalidate", // rather than rebuild - marking stale is O(1) per edit, where eager re-rendering turns a bulk import into a renderer stampede
		Label:   "On product change",
		Options: []config.Option{
			{Value: "off", Label: "Off - ignore product changes"},
			{Value: "invalidate", Label: "Invalidate - mark stale, rebuild on next visit"},
			{Value: "rebuild", Label: "Rebuild - re-render now via the queue"},
		},
	})
	config.Register(config.Definition{Path: CacheProductOnUpdate, Type: config.TypeBool, Default: "true", Label: "…when a product is updated (incl. price)"})
	config.Register(config.Definition{Path: CacheProductOnStock, Type: config.TypeBool, Default: "true", Label: "…when stock changes"})
	config.Register(config.Definition{Path: CacheProductOnDelete, Type: config.TypeBool, Default: "true", Label: "…when a product is deleted (always invalidates)"})
}
