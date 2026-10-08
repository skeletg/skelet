package front

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
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

// CmsPage returns a single published page (drafts hidden) that has an active listing on the request's store - a storeless request sees nothing, so a missing or bogus channel header can't fall back to the global flag and surface pages listed nowhere
func (r *queryResolver) CmsPage(ctx context.Context, slug *string, id *string, urlPath *string) (*model.CMSPage, error) {
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return nil, nil
	}
	loc := r.activeLocale(ctx)
	var (
		p   cms.Page
		err error
	)
	switch {
	case id != nil && *id != "":
		uid, perr := uuid.Parse(*id)
		if perr != nil {
			return nil, invalidID("page")
		}
		p, err = r.CMSSvc.GetPublishedPageForChannel(ctx, uid, *storeID, loc)
	case urlPath != nil && *urlPath != "":
		p, err = r.CMSSvc.GetPublishedPageByURLPathForChannel(ctx, *urlPath, *storeID, loc)
	case slug != nil && *slug != "":
		p, err = r.CMSSvc.GetPublishedPageBySlugForChannel(ctx, *slug, *storeID, loc)
	default:
		return nil, selectorRequired("slug, id, or urlPath is required")
	}
	if err != nil {
		return nil, err
	}
	if !p.IsPublished {
		return nil, nil
	}
	return r.frontPageModel(ctx, p), nil
}

func (r *queryResolver) CmsPages(ctx context.Context, filter *model.CMSPageFilter, limit *int32, offset *int32) ([]*model.CMSPage, error) {
	// Always the channel resolved from the request - filter.ChannelID is ignored here, matching CmsPage, so a public caller can't pick another channel and enumerate its pages, and a storeless one gets nothing
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return []*model.CMSPage{}, nil
	}
	f := cms.PageListFilter{
		OnlyPublished: true,
		ChannelID:     storeID,
	}
	f.Limit, f.Offset = cmsPageWindow(limit, offset)
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
	defaults := r.cmsTypeDefaults(ctx)
	prep := r.newCMSBlockPrep(ctx)
	out := make([]*model.CMSPage, len(pages))
	for i, p := range pages {
		prep.page(ctx, &p)
		out[i] = toFrontCMSPageModel(p, storeID, defaults)
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
	return r.frontPageModel(ctx, p), nil // same block prep as the public cmsPage resolver - sanitize html blocks, expand refs and so on
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
	return r.frontPageModel(ctx, p), nil
}

func (r *Resolver) CMSPage() CMSPageResolver { return &cMSPageResolver{r} }

type cMSPageResolver struct{ *Resolver }
