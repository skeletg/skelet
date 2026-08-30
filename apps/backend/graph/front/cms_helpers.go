package front

// Helpers live out of cms.resolvers.go so gqlgen's regen doesn't bury them in a comment block

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/blockkit"
	"github.com/skeletg/skelet/internal/cms"
	"github.com/skeletg/skelet/internal/urlrewrite"
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

func (r *queryResolver) frontPageModel(ctx context.Context, p cms.Page) *model.CMSPage { // for the single-page resolvers - the list resolver batches the type lookup itself
	m := toCMSPageModel(p)
	if pt, err := r.CMSSvc.GetPageType(ctx, p.PageTypeID); err == nil {
		applyPageTypeSEODefaults(pt, m)
	}
	return m
}

func (r *queryResolver) cmsPageForChannel( // centralized so all three lookup variants (id/slug/url) make the same global-vs-store-scoped choice
	ctx context.Context,
	storeID *uuid.UUID,
	global func(context.Context) (cms.Page, error),
	scoped func(context.Context, uuid.UUID) (cms.Page, error),
) (cms.Page, error) {
	if storeID != nil {
		return scoped(ctx, *storeID)
	}
	return global(ctx)
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

// On a block document: expand `ref` snippets, render admin custom block types to sanitized HTML, sanitize `html` blocks, resolve internal-link props to channel-aware URLs - freeform bodies pass through untouched
func (r *queryResolver) prepareCMSBlocks(ctx context.Context, p *cms.Page) {
	snippets := r.cmsSnippetResolver(ctx)
	custom := r.cmsCustomRenderer(ctx)
	links := r.cmsLinkResolver(ctx)
	assets := r.cmsAssetResolver(ctx)
	forms := r.cmsFormResolver(ctx)
	ec := r.cmsEvalContext(ctx)
	if p.Content != nil {
		p.Content.Body = preparedCMSBody(p.Content.Body, snippets, custom, links, assets, forms, ec)
	}
	for i := range p.AllContent {
		p.AllContent[i].Body = preparedCMSBody(p.AllContent[i].Body, snippets, custom, links, assets, forms, ec)
	}
	// A/B treatment bodies for the rendered locale, prepared exactly like the control body
	if p.ExperimentKey != "" && p.Content != nil {
		if vs, err := r.CMSSvc.GetPageVariants(ctx, p.ID, p.Content.Locale); err == nil {
			for i := range vs {
				vs[i].Body = preparedCMSBody(vs[i].Body, snippets, custom, links, assets, forms, ec)
			}
			p.Variants = vs
		}
	}
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

// The per-request audience for server-side block visibility - anonymous visitors get an empty segment set, so segment-allowlisted blocks hide; v1 targets logged-in customers only
func (r *queryResolver) cmsEvalContext(ctx context.Context) blockkit.EvalContext {
	ec := blockkit.EvalContext{Now: time.Now(), Segments: map[string]bool{}}
	if cid := auth.OptionalCustomerID(ctx); cid != nil && r.SegmentSvc != nil {
		if slugs, err := r.SegmentSvc.SlugsForCustomer(ctx, *cid); err == nil {
			for _, s := range slugs {
				ec.Segments[s] = true
			}
		}
	}
	return ec
}

func (r *queryResolver) cmsSnippetResolver(ctx context.Context) blockkit.SnippetResolver {
	return func(slug string) ([]blockkit.Block, bool) {
		return r.CMSSvc.GetSnippetBlocks(ctx, slug)
	}
}

// Interpolates instance props into the block type's HTML template - lazy, only hitting the DB when a custom non-builtin block is actually present
func (r *queryResolver) cmsCustomRenderer(ctx context.Context) blockkit.CustomRenderer {
	return func(typeKey string, props map[string]any) (string, bool) {
		bt, ok := r.CMSSvc.GetBlockTypeByKey(ctx, typeKey)
		if !ok {
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

func preparedCMSBody(raw json.RawMessage, snippets blockkit.SnippetResolver, custom blockkit.CustomRenderer, links blockkit.LinkResolver, assets blockkit.AssetResolver, forms blockkit.FormResolver, ec blockkit.EvalContext) json.RawMessage {
	doc, ok := blockkit.ParseDocument(raw)
	if !ok {
		return raw
	}
	prepared := blockkit.PrepareForFront(doc, snippets, custom, links, assets, forms)
	// Blocks hidden by schedule/segment for this viewer drop and the survivors lose their audience info - device rules ride through for client CSS
	prepared = blockkit.FilterServerSide(prepared, ec)
	out, err := json.Marshal(prepared)
	if err != nil {
		return raw
	}
	return out
}
