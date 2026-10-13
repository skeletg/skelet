package blockkit

import (
	"html"
	"maps"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Safe HTML rendering for admin-authored `html` blocks and, later, custom block types
// This is the SOLE XSS boundary - the storefront renders the output through dangerouslySetInnerHTML without re-sanitizing, so it has to be a real allowlist parser, not a regex blocklist that `<img src=x/onerror=...>` trivially bypasses

var tokenRe = regexp.MustCompile(`\{\{\s*([\w.]+)\s*\}\}`)

var htmlPolicy = func() *bluemonday.Policy { // built once, since bluemonday policies are goroutine-safe after construction - UGCPolicy allows common formatting and media tags but strips <script>, <style>, ALL event handlers and unsafe URL schemes, permits no <iframe>, <object> or <embed>, and `class` is added back so authored markup keeps its styling hooks
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class").Globally()
	// The merchant's own content, not strangers' comments - its links are followed like the rest of the site, and may open a new tab
	p.RequireNoFollowOnLinks(false)
	p.AllowAttrs("target").Matching(regexp.MustCompile(`^_blank$`)).OnElements("a")
	return p
}()

func SanitizeHTML(s string) string { // an allowlist-sanitized copy of admin-authored HTML, safe to render raw on the storefront
	return htmlPolicy.Sanitize(s)
}

func InterpolateEscaped(s string, vars map[string]string) string { // {{token}} becomes the HTML-escaped var value while the admin-trusted surrounding template stays intact - used for custom block types' HTML templates
	return tokenRe.ReplaceAllStringFunc(s, func(m string) string {
		key := strings.TrimSpace(tokenRe.FindStringSubmatch(m)[1])
		return html.EscapeString(vars[key])
	})
}

type SnippetResolver func(slug string) ([]Block, bool)

type CustomRenderer func(typeKey string, props map[string]any) (htmlOut string, ok bool) // renders an admin-defined custom block type to safe HTML from its instance field values

type LinkResolver func(entityType, entityID string) (url string, ok bool) // internal-link reference to a front-facing URL - ok=false when the entity has no URL, e.g. deleted, and the link prop is dropped; injected so blockkit stays free of a url_rewrites dependency

type ImageDescriptor struct { // the render-ready form of a media-asset reference - public URL plus intrinsic dimensions, so the storefront can emit width and height and avoid layout shift; Alt is the asset's default and resolveAssetProps applies any per-usage override
	Src    string
	Alt    string
	Width  int
	Height int
}

type AssetResolver func(assetID string) (ImageDescriptor, bool) // ok=false for a missing or deleted asset, in which case the image prop is dropped and the block renders without it - injected so blockkit stays free of a medialibrary dependency

type FormResolver func(formID string) (map[string]any, bool) // a web-form id to its storefront-safe definition (fields, display settings, captcha site key), inlined into the block's props so the storefront needs no second round-trip - ok=false for a missing or inactive form drops the block, and it's injected so blockkit stays free of a webform dependency

// The storefront's one entry point. Visibility is evaluated before preparing, which rebuilds some blocks and expands snippets, and again after for the snippets' own blocks
func ForFront(doc Document, ec EvalContext, snippets SnippetResolver, custom CustomRenderer, links LinkResolver, assets AssetResolver, forms FormResolver) Document {
	return FilterServerSide(PrepareForFront(FilterServerSide(doc, ec), snippets, custom, links, assets, forms), ec)
}

func PrepareForFront(doc Document, snippets SnippetResolver, custom CustomRenderer, links LinkResolver, assets AssetResolver, forms FormResolver) Document { // a render-ready copy for the storefront - sanitizes `html` blocks, expands `ref` snippets, renders custom block types into a safe `html` block, resolves internal-link props to URLs and media-asset props to image descriptors; pass nil for any seam to skip it
	return Document{Version: doc.Version, Blocks: prepareBlocks(doc.Blocks, snippets, custom, links, assets, forms, map[string]bool{}, 1)}
}

func prepareBlocks(in []Block, snippets SnippetResolver, custom CustomRenderer, links LinkResolver, assets AssetResolver, forms FormResolver, seen map[string]bool, depth int) []Block {
	if depth > maxDepth {
		return nil
	}
	out := make([]Block, 0, len(in))
	for _, b := range in {
		switch {
		case b.Type == TypeRef && snippets != nil:
			slug, _ := b.Props["slug"].(string)
			if slug == "" || seen[slug] {
				continue // missing or cyclic ref → drop
			}
			if blocks, ok := snippets(slug); ok {
				seen[slug] = true
				out = append(out, withVisibility(prepareBlocks(blocks, snippets, custom, links, assets, forms, seen, depth+1), b.Props)...)
				delete(seen, slug)
			}
			continue
		case b.Type == TypeHTML:
			nb := b
			nb.Props = sanitizeHTMLProp(b.Props)
			nb.Blocks = nil // an html block renders only its markup, so children would ship unused and unsanitized
			out = append(out, nb)
			continue
		case b.Type == TypeForm && forms != nil:
			formID, _ := b.Props["formId"].(string)
			if formID == "" {
				continue
			}
			data, ok := forms(formID)
			if !ok {
				continue // deleted / inactive → drop the block
			}
			out = append(out, Block{ID: b.ID, Type: TypeForm, Props: keepVisibility(map[string]any{"formId": formID, "form": data}, b.Props)})
			continue
		case !BuiltinTypes[b.Type] && custom != nil:
			if rendered, ok := custom(b.Type, b.Props); ok {
				out = append(out, Block{ID: b.ID, Type: TypeHTML, Props: keepVisibility(map[string]any{"html": SanitizeHTML(rendered)}, b.Props)}) // sanitize here too - the sole XSS boundary can't depend on every CustomRenderer remembering to sanitize its own output
				continue
			}
		}
		if links != nil {
			b.Props = resolveLinkProps(b.Props, links)
		}
		if assets != nil {
			b.Props = resolveAssetProps(b.Props, assets)
		}
		if len(b.Blocks) > 0 {
			b.Blocks = prepareBlocks(b.Blocks, snippets, custom, links, assets, forms, seen, depth+1)
		}
		out = append(out, b)
	}
	return out
}

func resolveLinkProps(props map[string]any, links LinkResolver) map[string]any { // a prop holding {type:"reference", entityType, entityId} becomes the resolved URL string - unresolvable references are dropped so the consuming block hides the link, and non-reference props pass through untouched
	if props == nil {
		return props
	}
	cp := make(map[string]any, len(props))
	for k, v := range props {
		if m, ok := v.(map[string]any); ok {
			if t, _ := m["type"].(string); t == "reference" {
				et, _ := m["entityType"].(string)
				id, _ := m["entityId"].(string)
				if url, ok := links(et, id); ok && url != "" {
					cp[k] = url
				}
				continue // resolved → set above; unresolved → drop the prop
			}
		}
		cp[k] = v
	}
	return cp
}

func resolveAssetProps(props map[string]any, assets AssetResolver) map[string]any { // {type:"asset", assetId, alt?, focalX?, focalY?} becomes {src, alt, width, height, focalX?, focalY?} and unresolvable references are dropped, mirroring resolveLinkProps - it recurses into repeater-item arrays so per-item refs in a gallery, logo row or feature grid resolve too, and plain string image props (external or hand-set URLs) pass through
	if props == nil {
		return props
	}
	cp := make(map[string]any, len(props))
	for k, v := range props {
		switch val := v.(type) {
		case map[string]any:
			if desc, isRef, ok := tryResolveAsset(val, assets); isRef {
				if ok {
					cp[k] = desc
				}
				continue // resolved → set above; unresolved → drop the prop
			}
			cp[k] = v
		case []any:
			out := make([]any, 0, len(val))
			for _, item := range val {
				if m, ok := item.(map[string]any); ok {
					out = append(out, resolveAssetProps(m, assets))
				} else {
					out = append(out, item)
				}
			}
			cp[k] = out
		default:
			cp[k] = v
		}
	}
	return cp
}

func tryResolveAsset(m map[string]any, assets AssetResolver) (desc map[string]any, isRef, ok bool) { // the per-usage `alt` overrides the asset's default, and any focal point on the ref carries through verbatim
	if t, _ := m["type"].(string); t != "asset" {
		return nil, false, false
	}
	id, _ := m["assetId"].(string)
	if id == "" {
		return nil, true, false
	}
	d, found := assets(id)
	if !found || d.Src == "" {
		return nil, true, false
	}
	out := map[string]any{"src": d.Src, "alt": d.Alt, "width": d.Width, "height": d.Height}
	if alt, ok := m["alt"].(string); ok && alt != "" {
		out["alt"] = alt
	}
	if fx, ok := m["focalX"]; ok {
		out["focalX"] = fx
	}
	if fy, ok := m["focalY"]; ok {
		out["focalY"] = fy
	}
	return out, true, true
}

func sanitizeHTMLProp(props map[string]any) map[string]any {
	cp := make(map[string]any, len(props))
	maps.Copy(cp, props)
	if raw, ok := cp["html"].(string); ok {
		cp["html"] = SanitizeHTML(raw)
	}
	return cp
}

// A rebuilt block keeps its visibility rule - without it the schedule and audience check after preparing would find nothing to enforce
func keepVisibility(props, from map[string]any) map[string]any {
	if v, ok := from["visibility"]; ok {
		props["visibility"] = v
	}
	return props
}

// A snippet's blocks inherit the ref's rule where they have none of their own, so the device hint set on the ref still reaches them
func withVisibility(blocks []Block, refProps map[string]any) []Block {
	v, ok := refProps["visibility"]
	if !ok {
		return blocks
	}
	for i := range blocks {
		if _, own := blocks[i].Props["visibility"]; !own {
			props := make(map[string]any, len(blocks[i].Props)+1)
			maps.Copy(props, blocks[i].Props)
			props["visibility"] = v
			blocks[i].Props = props
		}
	}
	return blocks
}
