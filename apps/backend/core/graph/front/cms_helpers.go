package front

// Helpers live out of cms.resolvers.go so gqlgen's regen doesn't bury them in a comment block

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/blockkit"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
)

func applyPageTypeSEODefaults(pt cms.PageType, m *model.CMSPage) { // read-time inheritance - the page's own value always wins
	if pt.DefaultSEONoindex {
		m.SeoNoindex = true
	}
	if pt.DefaultMetaTitle != "" && m.Content != nil && (m.Content.MetaTitle == nil || *m.Content.MetaTitle == "") {
		mt := strings.ReplaceAll(pt.DefaultMetaTitle, "{title}", m.Content.Title)
		m.Content.MetaTitle = &mt
	}
}

const (
	cmsPagesDefaultLimit = 50
	cmsPagesMaxLimit     = 100
)

func cmsPageWindow(limit, offset *int32) (int32, int32) {
	l := intDeref(limit, cmsPagesDefaultLimit)
	switch {
	case l <= 0:
		l = cmsPagesDefaultLimit
	case l > cmsPagesMaxLimit:
		l = cmsPagesMaxLimit
	}
	return l, max(intDeref(offset, 0), 0)
}

func (r *queryResolver) frontPageModel(ctx context.Context, p cms.Page) *model.CMSPage { // for the single-page resolvers - prepares the blocks and applies the type's SEO defaults
	r.newCMSBlockPrep(ctx).page(ctx, &p)
	return toFrontCMSPageModel(p, channelIDFromContext(ctx), r.cmsTypeDefaults(ctx))
}

func (r *queryResolver) cmsTypeDefaults(ctx context.Context) map[uuid.UUID]cms.PageType {
	defaults, err := r.CMSSvc.PageTypeSEODefaults(ctx)
	if err != nil {
		return nil
	}
	return defaults
}

func toFrontCMSPageModel(p cms.Page, channelID *uuid.UUID, defaults map[uuid.UUID]cms.PageType) *model.CMSPage {
	m := toCMSPageModel(p)
	if channelID != nil {
		for _, l := range p.ChannelListings {
			if l.ChannelID == *channelID && l.SEONoindex {
				m.SeoNoindex = true
			}
		}
	}
	if pt, ok := defaults[p.PageTypeID]; ok {
		applyPageTypeSEODefaults(pt, m)
	}
	return m
}

// The seam the seo.canonical / seo.nofollow / seo.jsonld resolver fields read, mirroring how ogImage reads seo.ogImage
func cmsSeoMetaValue(obj *model.CMSPage, key string) string {
	for _, m := range obj.Metadata {
		if m != nil && m.Key == key {
			return strings.TrimSpace(m.Value)
		}
	}
	return ""
}

// Operator-set metadata["seo.ogImage"] first, else the first image URL in the already asset-resolved body - nil when neither exists, so the storefront omits og:image
func cmsOgImage(obj *model.CMSPage) *string {
	for _, m := range obj.Metadata {
		if m != nil && m.Key == "seo.ogImage" {
			if v := strings.TrimSpace(m.Value); v != "" {
				return &v
			}
		}
	}
	if obj.Content != nil {
		if url := firstBodyImage(obj.Content.Body); url != "" {
			return &url
		}
	}
	return nil
}

// Checks known image prop keys per block in a stable order, plus repeater item arrays (gallery/logos/feature_grid), bounded by maxDepth=8
func firstBodyImage(body any) string {
	doc, ok := body.(map[string]any)
	if !ok {
		return ""
	}
	blocks, _ := doc["blocks"].([]any)
	return firstImageInBlocks(blocks, 1)
}

func firstImageInBlocks(blocks []any, depth int) string {
	if depth > 8 {
		return ""
	}
	for _, b := range blocks {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if props, ok := bm["props"].(map[string]any); ok {
			if url := imageInProps(props); url != "" {
				return url
			}
		}
		if child, ok := bm["blocks"].([]any); ok {
			if url := firstImageInBlocks(child, depth+1); url != "" {
				return url
			}
		}
	}
	return ""
}

func imageInProps(props map[string]any) string {
	for _, k := range []string{"imageUrl", "image", "src", "backgroundImage", "logo"} {
		if url := imageURLValue(props[k]); url != "" {
			return url
		}
	}
	if items, ok := props["items"].([]any); ok {
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				if url := imageInProps(m); url != "" {
					return url
				}
			}
		}
	}
	return ""
}

// After asset resolution an image prop is either a plain string or a descriptor map carrying "src"
func imageURLValue(v any) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case map[string]any:
		if s, ok := val["src"].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func toCMSPageModel(p cms.Page) *model.CMSPage {
	m := &model.CMSPage{
		ID:              p.ID.String(),
		PageTypeID:      p.PageTypeID.String(),
		Slug:            p.Slug,
		URLPath:         p.URLPath,
		Position:        p.Position,
		IsPublished:     p.IsPublished,
		SeoNoindex:      p.SEONoindex,
		PublishedAt:     p.PublishedAt,
		Data:            unmarshalJSON(p.Data),
		Metadata:        cmsMetadataItems(p.Metadata),
		PrivateMetadata: cmsMetadataItems(p.PrivateMetadata),
		Tags:            p.Tags,
		AllContent:      toCMSPageContentModels(p.AllContent),
		Selections:      toCMSPageSelectionsModel(p.Selections),
		Relationships:   toCMSPageRelationshipsModel(p.Relationships),
		ExperimentKey:   p.ExperimentKey,
		Variants:        toCMSPageVariantModels(p.Variants),
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
	if p.ParentID != nil {
		id := p.ParentID.String()
		m.ParentID = &id
	}
	if p.Content != nil {
		m.Content = toCMSPageContentModel(*p.Content)
	}
	return m
}

func toCMSPageVariantModels(vs []cms.PageVariant) []*model.CMSPageVariant {
	out := make([]*model.CMSPageVariant, len(vs))
	for i, v := range vs {
		out[i] = &model.CMSPageVariant{VariantKey: v.VariantKey, Body: unmarshalJSON(v.Body)}
	}
	return out
}

// The per-request block preparation seams, built once and shared by every page a request renders - snippets and custom block types are memoized, so a list doesn't reload the same one per page
type cmsBlockPrep struct {
	r        *queryResolver
	snippets blockkit.SnippetResolver
	custom   blockkit.CustomRenderer
	links    blockkit.LinkResolver
	assets   blockkit.AssetResolver
	forms    blockkit.FormResolver
	ec       blockkit.EvalContext
}

func (r *queryResolver) newCMSBlockPrep(ctx context.Context) *cmsBlockPrep {
	return &cmsBlockPrep{
		r:        r,
		snippets: r.cmsSnippetResolver(ctx),
		custom:   r.cmsCustomRenderer(ctx),
		links:    r.cmsLinkResolver(ctx),
		assets:   r.cmsAssetResolver(ctx),
		forms:    r.cmsFormResolver(ctx),
		ec:       r.cmsEvalContext(ctx),
	}
}

// On a block document: expand `ref` snippets, render admin custom block types to sanitized HTML, sanitize `html` blocks, resolve internal-link props to channel-aware URLs - freeform bodies pass through untouched
func (bp *cmsBlockPrep) page(ctx context.Context, p *cms.Page) {
	if p.Content == nil {
		return
	}
	p.Content.Body = bp.body(p.Content.Body)
	if p.ExperimentKey != "" {
		if vs, err := bp.r.CMSSvc.GetPageVariants(ctx, p.ID, p.Content.Locale); err == nil {
			for i := range vs {
				vs[i].Body = bp.body(vs[i].Body)
			}
			p.Variants = vs
		}
	}
}

func (bp *cmsBlockPrep) body(raw json.RawMessage) json.RawMessage {
	return preparedCMSBody(raw, bp.snippets, bp.custom, bp.links, bp.assets, bp.forms, bp.ec)
}

// Inlines the referenced form so the storefront renders it without a second round-trip, and ok=false (missing, inactive, cross-channel) drops the block - lazy, so it only hits the DB when a form block is actually present
func (r *queryResolver) cmsFormResolver(ctx context.Context) blockkit.FormResolver {
	return func(formID string) (map[string]any, bool) {
		if r.WebFormSvc == nil {
			return nil, false
		}
		fid, err := uuid.Parse(formID)
		if err != nil {
			return nil, false
		}
		pf, err := r.WebFormSvc.PublicForm(ctx, fid)
		if err != nil {
			return nil, false
		}
		// Forms are per-channel, so this fails closed on a channel-less request rather than inlining another channel's form
		sid := channelIDFromContext(ctx)
		if sid == nil || pf.ChannelID != *sid {
			return nil, false
		}
		return map[string]any{
			"id":              pf.ID.String(),
			"name":            pf.Name,
			"slug":            pf.Slug,
			"fields":          pf.Fields,
			"settings":        pf.Settings,
			"captchaProvider": pf.CaptchaProvider,
			"captchaSiteKey":  pf.CaptchaSiteKey,
		}, true
	}
}

// The audience read CMS block visibility needs - a one-method seam, so the front API doesn't import Synapse for it
type CustomerSegments interface {
	SlugsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error)
}

// The per-request audience for server-side block visibility - anonymous visitors get an empty segment set, so segment-allowlisted blocks hide; v1 targets logged-in customers only
func (r *queryResolver) cmsEvalContext(ctx context.Context) blockkit.EvalContext {
	ec := blockkit.EvalContext{Now: time.Now(), Segments: map[string]bool{}}
	if cid := channel.OptionalCustomerID(ctx); cid != nil && r.SegmentSvc != nil {
		if slugs, err := r.SegmentSvc.SlugsForCustomer(ctx, *cid); err == nil {
			for _, s := range slugs {
				ec.Segments[s] = true
			}
		}
	}
	return ec
}

func (r *queryResolver) cmsSnippetResolver(ctx context.Context) blockkit.SnippetResolver {
	type hit struct {
		blocks []blockkit.Block
		ok     bool
	}
	seen := map[string]hit{}
	return func(slug string) ([]blockkit.Block, bool) {
		if h, ok := seen[slug]; ok {
			return h.blocks, h.ok
		}
		blocks, ok := r.CMSSvc.GetSnippetBlocks(ctx, slug)
		seen[slug] = hit{blocks, ok}
		return blocks, ok
	}
}

// Interpolates instance props into the block type's HTML template - lazy, only hitting the DB when a custom non-builtin block is actually present
func (r *queryResolver) cmsCustomRenderer(ctx context.Context) blockkit.CustomRenderer {
	types := map[string]*cms.BlockType{}
	return func(typeKey string, props map[string]any) (string, bool) {
		bt, seen := types[typeKey]
		if !seen {
			if found, ok := r.CMSSvc.GetBlockTypeByKey(ctx, typeKey); ok {
				bt = &found
			}
			types[typeKey] = bt
		}
		if bt == nil {
			return "", false
		}
		vars := make(map[string]string, len(props))
		for k, v := range props {
			if s, isStr := v.(string); isStr {
				vars[k] = s
			} else if v != nil {
				vars[k] = fmt.Sprintf("%v", v)
			}
		}
		return blockkit.SanitizeHTML(blockkit.InterpolateEscaped(bt.HTMLTemplate, vars)), true
	}
}

// Bound to the request's channel - an internal link resolves to that channel's canonical URL, falling back to the global one, so CMS links follow slug renames and localize per channel, while unknown or deleted entities resolve ok=false and the link drops
func (r *queryResolver) cmsLinkResolver(ctx context.Context) blockkit.LinkResolver {
	channelID := channelIDFromContext(ctx)
	return func(entityType, entityID string) (string, bool) {
		id, err := uuid.Parse(entityID)
		if err != nil {
			return "", false
		}
		path, ok := r.URLRewriteSvc.CanonicalPathForChannel(ctx, urlrewrite.EntityType(entityType), id, channelID)
		if !ok || path == "" {
			return "", false
		}
		return "/" + path, true
	}
}

// Resolves {type:"asset",assetId} to a public image descriptor, memoized per request so repeated and missing ids cost one lookup per page render
// nil when the media service is unavailable - asset resolution is then skipped and unresolved refs drop; v1 resolves per-id via GetAsset, and batching through GetAssetsByIDs after a pre-scan for ids is still a follow-up
func (r *queryResolver) cmsAssetResolver(ctx context.Context) blockkit.AssetResolver {
	if r.MediaLibrarySvc == nil {
		return nil
	}
	cache := map[string]blockkit.ImageDescriptor{}
	miss := map[string]bool{}
	return func(assetID string) (blockkit.ImageDescriptor, bool) {
		if d, ok := cache[assetID]; ok {
			return d, true
		}
		if miss[assetID] {
			return blockkit.ImageDescriptor{}, false
		}
		id, err := uuid.Parse(assetID)
		if err != nil {
			miss[assetID] = true
			return blockkit.ImageDescriptor{}, false
		}
		a, err := r.MediaLibrarySvc.GetAsset(ctx, id)
		if err != nil || a.URL == "" {
			miss[assetID] = true
			return blockkit.ImageDescriptor{}, false
		}
		d := blockkit.ImageDescriptor{Src: a.URL, Alt: a.AltText}
		if a.Width != nil {
			d.Width = int(*a.Width)
		}
		if a.Height != nil {
			d.Height = int(*a.Height)
		}
		cache[assetID] = d
		return d, true
	}
}

// A body claiming to be a block document that doesn't parse is dropped rather than passed through - the storefront would otherwise render its blocks unsanitized
func preparedCMSBody(raw json.RawMessage, snippets blockkit.SnippetResolver, custom blockkit.CustomRenderer, links blockkit.LinkResolver, assets blockkit.AssetResolver, forms blockkit.FormResolver, ec blockkit.EvalContext) json.RawMessage {
	doc, ok := blockkit.ParseDocument(raw)
	if !ok {
		if blockkit.ClaimsDocument(raw) {
			return nil
		}
		return raw
	}
	// Blocks hidden by schedule/segment for this viewer drop and the survivors lose their audience info - device rules ride through for client CSS
	prepared := blockkit.ForFront(doc, ec, snippets, custom, links, assets, forms)
	out, err := json.Marshal(prepared)
	if err != nil {
		return nil
	}
	return out
}
