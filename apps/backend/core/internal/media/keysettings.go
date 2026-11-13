package media

import (
	"context"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// The top-level prefix everything the catalog uploads sits under, which the media library keeps out of
const CatalogRoot = "catalog"

type KeySetting struct {
	Path    string
	Default string
	Folders bool
	Slots   []string
	label   string
	about   string
}

type KeyConfig interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

const keyShared = "{name} is the uploaded file's name, {yyyy} {mm} {dd} the upload date, and it needs {id} or {short_id} so an address is never used twice, with {ext} at the end."

var (
	LibraryFileKey = KeySetting{
		Path: "system/media/library_file", Default: "{id}-{name}{ext}", label: "Library file names",
		about: "How a file uploaded to the media library is named inside its folder. " + keyShared + " Files already there keep their address until they're moved or renamed.",
	}
	ProductImageKey = KeySetting{
		Path: "system/media/product_image", Default: "products/{product_id}/{type}/{id}{ext}", Folders: true, Slots: []string{"product_id", "sku", "type"},
		label: "Product image paths",
		about: "Where a product's pictures go under catalog/, folders included. Takes {product_id}, {sku} and {type} (gallery, main, thumbnail, og, description or short-description) on top of the library's placeholders. Pictures already uploaded stay where they are.",
	}
	CategoryCoverKey = KeySetting{
		Path: "system/media/category_cover", Default: "categories/{category_id}/cover/{id}{ext}", Folders: true, Slots: []string{"category_id", "slug"},
		label: "Category cover paths",
		about: "Where a category's cover goes under catalog/. Takes {category_id} and {slug} on top of the library's placeholders.",
	}
	CollectionCoverKey = KeySetting{
		Path: "system/media/collection_cover", Default: "collections/{collection_id}/cover/{id}{ext}", Folders: true, Slots: []string{"collection_id", "slug"},
		label: "Collection cover paths",
		about: "Where a collection's cover goes under catalog/. Takes {collection_id} and {slug} on top of the library's placeholders.",
	}
)

var keyDefaults = map[string]KeyTemplate{}

func init() {
	config.RegisterGroup(config.Group{Section: "system", Slug: "media", Label: "Media files", SortOrder: 5})
	for _, k := range []KeySetting{LibraryFileKey, ProductImageKey, CategoryCoverKey, CollectionCoverKey} {
		t, err := k.Parse(k.Default)
		if err != nil {
			panic("media: default template for " + k.Path + ": " + err.Error())
		}
		keyDefaults[k.Path] = t
		config.Register(config.Definition{
			Path: k.Path, Type: config.TypeString, Default: k.Default, Label: k.label, Description: k.about,
			MaxLen: keyTemplateMax, Check: k.check,
		})
	}
}

func (k KeySetting) Parse(s string) (KeyTemplate, error) {
	return ParseKeyTemplate(s, k.Folders, k.Slots...)
}

// An empty value is the way back to the default, so it passes
func (k KeySetting) check(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	_, err := k.Parse(value)
	return err
}

// The layout in force - the default stands in when nothing is saved, the settings can't be read or what was saved no longer parses
func (k KeySetting) Template(ctx context.Context, cfg KeyConfig) KeyTemplate {
	if cfg == nil {
		return keyDefaults[k.Path]
	}
	saved, err := cfg.GetString(ctx, k.Path, config.DefaultScope)
	if err == nil && strings.TrimSpace(saved) == "" {
		return keyDefaults[k.Path]
	}
	if err == nil {
		var t KeyTemplate
		if t, err = k.Parse(saved); err == nil {
			return t
		}
	}
	applog.From(ctx).Warn("media: path template unusable, the default stands in", "path", k.Path, "err", err)
	return keyDefaults[k.Path]
}
