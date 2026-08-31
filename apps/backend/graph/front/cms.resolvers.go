package front

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/cms"
)

func (r *cMSPageResolver) OgImage(ctx context.Context, obj *model.CMSPage) (*string, error) {
	return cmsOgImage(obj), nil
}

func (r *cMSPageResolver) CanonicalURL(ctx context.Context, obj *model.CMSPage) (*string, error) {
	if v := cmsSeoMetaValue(obj, "seo.canonical"); v != "" { // operator override, else null so the storefront self-canonicalises
		return &v, nil
	}
	return nil, nil
}

func (r *cMSPageResolver) SeoNofollow(ctx context.Context, obj *model.CMSPage) (bool, error) {
	v := cmsSeoMetaValue(obj, "seo.nofollow") // robots nofollow directive, in metadata since it has no column
	return v == "1" || strings.EqualFold(v, "true"), nil
}

func (r *cMSPageResolver) StructuredData(ctx context.Context, obj *model.CMSPage) (*string, error) {
	if v := cmsSeoMetaValue(obj, "seo.jsonld"); v != "" { // raw operator-authored JSON-LD - the storefront validates before injecting
		return &v, nil
	}
	return nil, nil
}

func (r *cMSPageResolver) StructuredDataSpec(ctx context.Context, obj *model.CMSPage) (*string, error) {
	if v := cmsSeoMetaValue(obj, "seo.schema"); v != "" { // the builder spec the storefront turns into fresh JSON-LD
		return &v, nil
	}
	return nil, nil
}

// CmsPage returns a single published page (drafts hidden) - with a store in context it must also have an active listing on that store, so globally published but unlisted pages stay hidden
func (r *queryResolver) CmsPage(ctx context.Context, slug *string, id *string, urlPath *string) (*model.CMSPage, error) {
	loc := r.activeLocale(ctx)
	storeID := channelIDFromContext(ctx)
	gate := func(p cms.Page) (*model.CMSPage, error) {
		if !p.IsPublished {
			return nil, nil
		}
		r.prepareCMSBlocks(ctx, &p) // the per-store *ForChannel lookups already enforced the listing - the no-store fallback only checks the global flag
		return r.frontPageModel(ctx, p), nil
	}
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, invalidID("page")
		}
		p, err := r.cmsPageForChannel(ctx, storeID, func(c context.Context) (cms.Page, error) {
			return r.CMSSvc.GetPage(c, uid, loc)
		}, func(c context.Context, sv uuid.UUID) (cms.Page, error) {
			return r.CMSSvc.GetPublishedPageForChannel(c, uid, sv, loc)
		})
		if err != nil {
			return nil, err
		}
		return gate(p)
	case urlPath != nil && *urlPath != "":
		p, err := r.cmsPageForChannel(ctx, storeID, func(c context.Context) (cms.Page, error) {
			return r.CMSSvc.GetPageByURLPath(c, *urlPath, loc)
		}, func(c context.Context, sv uuid.UUID) (cms.Page, error) {
			return r.CMSSvc.GetPublishedPageByURLPathForChannel(c, *urlPath, sv, loc)
		})
		if err != nil {
			return nil, err
		}
		return gate(p)
	case slug != nil && *slug != "":
		p, err := r.cmsPageForChannel(ctx, storeID, func(c context.Context) (cms.Page, error) {
			return r.CMSSvc.GetPageBySlug(c, *slug, loc)
		}, func(c context.Context, sv uuid.UUID) (cms.Page, error) {
			return r.CMSSvc.GetPublishedPageBySlugForChannel(c, *slug, sv, loc)
		})
		if err != nil {
			return nil, err
		}
		return gate(p)
	}
	return nil, selectorRequired("slug, id, or urlPath is required")
}

func (r *queryResolver) CmsPages(ctx context.Context, filter *model.CMSPageFilter, limit *int32, offset *int32) ([]*model.CMSPage, error) {
	f := cms.PageListFilter{
		OnlyPublished: true,
		Limit:         intDeref(limit, 50),
		Offset:        intDeref(offset, 0),
	}
	// Always the channel resolved from the request - filter.ChannelID is ignored here, matching CmsPage, so a public caller can't pick another channel and enumerate its pages
	f.ChannelID = channelIDFromContext(ctx)
	if filter != nil {
		if v, err := parseUUIDOpt(filter.PageTypeID); err != nil {
			return nil, err
		} else {
			f.PageTypeID = v
		}
		if v, err := parseUUIDOpt(filter.ParentID); err != nil {
			return nil, err
		} else {
			f.ParentID = v
		}
	}
	pages, err := r.CMSSvc.ListPages(ctx, f, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	typeByID := map[uuid.UUID]cms.PageType{} // page-type defaults batched once, since the list can be large - the sitemap, say
	if types, err := r.CMSSvc.ListPageTypes(ctx); err == nil {
		for _, t := range types {
			typeByID[t.ID] = t
		}
	}
	out := make([]*model.CMSPage, len(pages))
	for i, p := range pages {
		r.prepareCMSBlocks(ctx, &p)
		m := toCMSPageModel(p)
		if pt, ok := typeByID[p.PageTypeID]; ok {
			applyPageTypeSEODefaults(pt, m)
		}
		out[i] = m
	}
	return out, nil
}

// Skips the published-and-listed gates CmsPage enforces - the preview token is the authorisation, and an unknown or expired one returns nil, which the dashboard reads as "session expired, refresh"
func (r *queryResolver) PreviewCMSPage(ctx context.Context, token string) (*model.CMSPage, error) {
	if r.CMSPreviewStore == nil {
		return nil, nil
	}
	claims, err := r.CMSPreviewStore.Validate(ctx, token)
	if err != nil {
		return nil, nil
	}
	loc := claims.Locale
	if loc == "" {
		loc = r.activeLocale(ctx)
	}
	p, err := r.CMSSvc.GetPage(ctx, claims.PostID, loc) // PostID is a generic entity-id claim field - the CMS mint stores the page id there
	if err != nil {
		return nil, nil
	}
	r.prepareCMSBlocks(ctx, &p) // same block prep as the public cmsPage resolver - sanitize html blocks, expand refs and so on
	return r.frontPageModel(ctx, p), nil
}

// The page flagged is_homepage on its listing for THIS channel - each channel designates its own, and nil (no store, or none set) means the storefront falls back to its built-in homepage
// The page stays reachable at its own url_path too, and the storefront catch-all redirects that path to / when it is the channel homepage
func (r *queryResolver) HomePage(ctx context.Context) (*model.CMSPage, error) {
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return nil, nil
	}
	p, err := r.CMSSvc.GetHomePageForChannel(ctx, *storeID, r.activeLocale(ctx))
	if err != nil || !p.IsPublished {
		return nil, nil
	}
	r.prepareCMSBlocks(ctx, &p)
	return r.frontPageModel(ctx, p), nil
}

func (r *Resolver) CMSPage() CMSPageResolver { return &cMSPageResolver{r} }

type cMSPageResolver struct{ *Resolver }
