// Package blockkit is the shared CMS page-layout engine - the typed block tree a page's body stores, its validation, and safe HTML rendering for admin-defined custom blocks
// The block `type` strings are a CONTRACT shared three ways: this validator, the dashboard palette in features/cms, and the storefront renderer registry in app/components/cms
// SYNC SENTINEL: keep BuiltinTypes in lockstep with those two lists - bin/contract-check fails the build when they drift
package blockkit

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const maxDepth = 8 // containers land in a later phase and v1 is flat, but the bound means a malformed or cyclic tree can't blow the stack

type Block struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Props  map[string]any `json:"props,omitempty"`
	Blocks []Block        `json:"blocks,omitempty"`
}

type Document struct { // what cms_page_content.body holds for a block-built page - a body that doesn't parse to one with blocks is freeform content, which the storefront falls back to plain text for
	Version int     `json:"version"`
	Blocks  []Block `json:"blocks"`
}

const ( // built-in block types for v1 - the dynamic ones (product, collection, category, blog) carry a source descriptor in Props and the storefront fetches their data
	TypeHero           = "hero"
	TypeRichText       = "rich_text"
	TypeProductGrid    = "product_grid"
	TypeCollectionGrid = "collection_grid"
	TypeCategoryGrid   = "category_grid"
	TypeBlogGrid       = "blog_grid"
	TypeCTABanner      = "cta_banner"
	TypeHTML           = "html"
	TypeRef            = "ref"
	TypeSection        = "section"
	TypeColumns        = "columns"
	TypeAccordion      = "accordion"
	TypeTabs           = "tabs"
	TypeVideo          = "video"
	TypeGallery        = "gallery"
	TypeDivider        = "divider"
	TypeQuote          = "quote"
	TypeStats          = "stats"
	TypeCallout        = "callout"
	TypeButtons        = "buttons"
	TypeMediaText      = "media_text"
	TypeFeatureGrid    = "feature_grid"
	TypeLogos          = "logos"
	TypeTimeline       = "timeline"
	TypeValueStrip     = "value_strip"
	TypePromoMosaic    = "promo_mosaic"
	TypeForm           = "form" // embeds a custom web form by id, resolved and inlined server-side
)

var BuiltinTypes = map[string]bool{
	TypeHero: true, TypeRichText: true, TypeProductGrid: true,
	TypeCollectionGrid: true, TypeCategoryGrid: true, TypeBlogGrid: true,
	TypeCTABanner: true, TypeHTML: true, TypeRef: true,
	TypeSection: true, TypeColumns: true,
	TypeAccordion: true, TypeTabs: true, TypeVideo: true, TypeGallery: true, TypeDivider: true,
	TypeQuote: true, TypeStats: true, TypeCallout: true, TypeButtons: true,
	TypeMediaText: true, TypeFeatureGrid: true, TypeLogos: true, TypeTimeline: true,
	TypeForm:       true,
	TypeValueStrip: true, TypePromoMosaic: true,
}

type CustomTypeChecker func(typeKey string) bool // reports whether a type string is an admin-defined custom block for the channel - a nil checker means no custom types

func CloneWithNewIDs(doc Document) Document { // regenerating every block ID lets a page-type template seed a new page without two pages sharing IDs - Props carry by reference, since they're value data serialized into the new body immediately after and the template is never mutated
	return Document{Version: doc.Version, Blocks: cloneBlocks(doc.Blocks)}
}

func cloneBlocks(in []Block) []Block {
	if in == nil {
		return nil
	}
	out := make([]Block, len(in))
	for i, b := range in {
		out[i] = Block{
			ID:     uuid.NewString(),
			Type:   b.Type,
			Props:  b.Props,
			Blocks: cloneBlocks(b.Blocks),
		}
	}
	return out
}

func ParseDocument(raw json.RawMessage) (doc Document, ok bool) {
	if len(raw) == 0 {
		return Document{}, false
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Document{}, false
	}
	return doc, doc.Blocks != nil || doc.Version > 0 // a block document is identified by a blocks array or an explicit version - plain rich-text and Lexical bodies are objects with neither
}

func Validate(doc Document, custom CustomTypeChecker) error { // every block needs a known type, built-in or a channel custom one, nesting within maxDepth, and an id
	return validateBlocks(doc.Blocks, custom, 1)
}

func validateBlocks(blocks []Block, custom CustomTypeChecker, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("block nesting exceeds %d levels", maxDepth)
	}
	for i, b := range blocks {
		if b.Type == "" {
			return fmt.Errorf("block %d has no type", i)
		}
		if !BuiltinTypes[b.Type] && (custom == nil || !custom(b.Type)) {
			return fmt.Errorf("unknown block type %q", b.Type)
		}
		if b.Type == TypeRef {
			if _, ok := b.Props["slug"].(string); !ok {
				return fmt.Errorf("ref block %d missing slug", i)
			}
		}
		if len(b.Blocks) > 0 {
			if err := validateBlocks(b.Blocks, custom, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
