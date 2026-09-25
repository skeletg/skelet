package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog/preview"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateCMSPage(ctx context.Context, input model.CreateCMSPageInput) (*model.CMSPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	in, err := fromCreateCMSPageInput(input)
	if err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.CreatePage(ctx, in)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) UpdateCMSPage(ctx context.Context, input model.UpdateCMSPageInput) (*model.CMSPage, error) {
	in, err := fromUpdateCMSPageInput(input)
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", in.ID, ""); err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.UpdatePage(ctx, in)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) DeleteCMSPage(ctx context.Context, id string) (bool, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", uid, ""); err != nil {
		return false, err
	}
	if err := r.CMSSvc.DeletePage(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CloneCMSPage(ctx context.Context, id string) (*model.CMSPage, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", uid, ""); err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.ClonePage(ctx, uid)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) BulkPublishCMSPages(ctx context.Context, ids []string, published bool) (*model.BulkCMSPageResult, error) {
	uids, err := parseCMSPageIDs(ids)
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPagesChannels(ctx, "cms.manage", uids); err != nil {
		return nil, err
	}
	n, err := r.CMSSvc.BulkSetPublished(ctx, uids, published)
	if err != nil {
		return nil, err
	}
	return &model.BulkCMSPageResult{Count: int32(n)}, nil
}

func (r *mutationResolver) BulkTagCMSPages(ctx context.Context, ids []string, addTags []string, removeTags []string) (*model.BulkCMSPageResult, error) {
	uids, err := parseCMSPageIDs(ids)
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPagesChannels(ctx, "cms.manage", uids); err != nil {
		return nil, err
	}
	n, err := r.CMSSvc.BulkTag(ctx, uids, addTags, removeTags)
	if err != nil {
		return nil, err
	}
	return &model.BulkCMSPageResult{Count: int32(n)}, nil
}

func (r *mutationResolver) BulkDeleteCMSPages(ctx context.Context, ids []string) (*model.BulkCMSPageResult, error) {
	uids, err := parseCMSPageIDs(ids)
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPagesChannels(ctx, "cms.manage", uids); err != nil {
		return nil, err
	}
	n, err := r.CMSSvc.BulkDelete(ctx, uids)
	if err != nil {
		return nil, err
	}
	return &model.BulkCMSPageResult{Count: int32(n)}, nil
}

// All-listed-channels gate: the page-content row is shared across every channel listing the page, so the admin must
// have cms.manage at the requested locale on every one of those channels (or a global grant)
func (r *mutationResolver) UpsertCMSPageContent(ctx context.Context, input model.UpsertCMSPageContentInput) (*model.CMSPage, error) {
	pid, err := parseID(input.PageID, "page")
	if err != nil {
		return nil, err
	}
	listings, err := r.CMSSvc.ListPageChannelListings(ctx, pid)
	if err != nil {
		return nil, err
	}
	channelIDs := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		channelIDs[i] = l.ChannelID
	}
	if err := auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, "cms.manage", channelIDs, input.Locale); err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.UpsertPageContent(ctx, pid, cms.PageContentInput{
		Locale:          input.Locale,
		Title:           input.Title,
		Body:            rawJSON(input.Body),
		MetaTitle:       ptr.Deref(input.MetaTitle),
		MetaDescription: ptr.Deref(input.MetaDescription),
		MetaKeywords:    ptr.Deref(input.MetaKeywords),
		Data:            rawJSON(input.Data),
	})
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) PublishCMSPage(ctx context.Context, id string) (*model.CMSPage, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", uid, ""); err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.PublishPage(ctx, uid)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) UnpublishCMSPage(ctx context.Context, id string) (*model.CMSPage, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", uid, ""); err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.UnpublishPage(ctx, uid)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) SetCMSPageSelections(ctx context.Context, pageID string, attributeID string, valueIds []string) (*model.CMSPage, error) {
	pid, err := parseID(pageID, "page")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", pid, ""); err != nil {
		return nil, err
	}
	aid, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	vids, err := parseUUIDs(valueIds)
	if err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.SetPageSelections(ctx, pid, aid, vids)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) SetCMSPageRelationships(ctx context.Context, pageID string, attributeID string, refs []*model.CMSRelationshipRefInput) (*model.CMSPage, error) {
	pid, err := parseID(pageID, "page")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", pid, ""); err != nil {
		return nil, err
	}
	aid, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	out := make([]cms.PageRelationship, 0, len(refs))
	for i, ref := range refs {
		tid, err := parseID(ref.TargetID, "target")
		if err != nil {
			return nil, err
		}
		out = append(out, cms.PageRelationship{
			AttributeID: aid,
			TargetType:  cms.RelationshipTarget(cmsTargetToString(ref.TargetType)),
			TargetID:    tid,
			Position:    int32(i),
		})
	}
	p, err := r.CMSSvc.SetPageRelationships(ctx, pid, aid, out)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) SetCMSPageExperiment(ctx context.Context, pageID string, experimentKey string) (*model.CMSPage, error) {
	pid, err := parseID(pageID, "page")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", pid, ""); err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.SetPageExperiment(ctx, pid, experimentKey)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) UpsertCMSPageVariantContent(ctx context.Context, pageID string, locale string, variantKey string, body interface{}) (*model.CMSPage, error) {
	pid, err := parseID(pageID, "page")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", pid, locale); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if body != nil {
		if raw, err = json.Marshal(body); err != nil {
			return nil, errInvalidInput("body", err)
		}
	}
	p, err := r.CMSSvc.UpsertPageVariantBody(ctx, pid, locale, variantKey, raw)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) SetCMSPageChannelListing(ctx context.Context, input model.SetCMSPageChannelListingInput) (*model.CMSPage, error) {
	pageID, err := parseID(input.PageID, "page")
	if err != nil {
		return nil, err
	}
	storeID, err := parseID(input.ChannelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := r.requirePageListingChange(ctx, pageID, storeID, false); err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.SetPageChannelListing(ctx, cms.SetPageChannelListingInput{
		PageID:      pageID,
		ChannelID:   storeID,
		IsPublished: input.IsPublished,
		SEONoindex:  boolDeref(input.SeoNoindex, false),
		IsHomepage:  boolDeref(input.IsHomepage, false),
	})
	if err != nil {
		return nil, err
	}
	m := toCMSPageModel(p)
	return m, r.visiblePageListings(ctx, m)
}

func (r *mutationResolver) DeleteCMSPageChannelListing(ctx context.Context, pageID string, channelID string) (*model.CMSPage, error) {
	pid, err := parseID(pageID, "page")
	if err != nil {
		return nil, err
	}
	sid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := r.requirePageListingChange(ctx, pid, sid, true); err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.DeletePageChannelListing(ctx, pid, sid)
	if err != nil {
		return nil, err
	}
	m := toCMSPageModel(p)
	return m, r.visiblePageListings(ctx, m)
}

// Mirror of MintBlogPostPreviewToken: gates on cms.view, confirms the page exists (so the mint endpoint can't be used to fish for valid
// page ids), then stores the page id + locale under a short-lived token the storefront /cms/preview route validates via previewCMSPage
func (r *mutationResolver) MintCMSPagePreviewToken(ctx context.Context, input model.MintCMSPagePreviewTokenInput) (*model.CMSPagePreviewToken, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	if r.CMSPreviewStore == nil {
		return nil, errNotConfigured("preview store")
	}
	pageID, err := uuid.Parse(input.PageID)
	if err != nil {
		return nil, errInvalidID("pageId", err)
	}
	// Confirm the page exists before minting - minting tokens for
	// non-existent pages is a leak vector.
	if _, err := r.CMSSvc.GetPage(ctx, pageID, ""); err != nil {
		return nil, err
	}
	if err := r.requireOnPageChannels(ctx, "cms.view", pageID, ""); err != nil {
		return nil, err
	}
	claims, ok := auth.AdminFromContext(ctx)
	if !ok || claims == nil {
		return nil, errAdminRequired()
	}
	loc := ""
	if input.Locale != nil {
		loc = *input.Locale
	}
	res, err := r.CMSPreviewStore.Mint(ctx, preview.MintInput{
		PostID:  pageID, // PostID is the generic entity id - holds the page id here.
		AdminID: claims.AdminID,
		Locale:  loc,
	})
	if err != nil {
		return nil, err
	}
	return &model.CMSPagePreviewToken{Token: res.Token, ExpiresAt: res.ExpiresAt}, nil
}

func (r *queryResolver) CmsPage(ctx context.Context, slug *string, id *string, urlPath *string) (*model.CMSPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	loc := r.activeLocale(ctx)
	gate := func(p cms.Page) (*model.CMSPage, error) {
		// Hydrate A/B variant bodies for the active locale so the editor can
		// populate its variant switcher.
		if p.ExperimentKey != "" && loc != "" {
			if vs, err := r.CMSSvc.GetPageVariants(ctx, p.ID, loc); err == nil {
				p.Variants = vs
			}
		}
		m := toCMSPageModel(p)
		return m, r.visiblePageListings(ctx, m)
	}
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, errInvalidID("id", err)
		}
		p, err := r.CMSSvc.GetPage(ctx, uid, loc)
		if err != nil {
			return nil, err
		}
		return gate(p)
	case urlPath != nil && *urlPath != "":
		p, err := r.CMSSvc.GetPageByURLPath(ctx, *urlPath, loc)
		if err != nil {
			return nil, err
		}
		return gate(p)
	case slug != nil && *slug != "":
		p, err := r.CMSSvc.GetPageBySlug(ctx, *slug, loc)
		if err != nil {
			return nil, err
		}
		return gate(p)
	}
	return nil, fmt.Errorf("slug, id, or urlPath is required")
}

func (r *queryResolver) CmsPages(ctx context.Context, filter *model.CMSPageFilter, limit *int32, offset *int32) ([]*model.CMSPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	f := cms.PageListFilter{
		OnlyPublished: true,
		Limit:         intDeref(limit, 50),
		Offset:        intDeref(offset, 0),
	}
	if filter != nil {
		if filter.OnlyPublished != nil {
			f.OnlyPublished = *filter.OnlyPublished
		}
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
		if v, err := parseUUIDOpt(filter.ChannelID); err != nil {
			return nil, err
		} else {
			f.ChannelID = v
		}
		if filter.WorkflowState != nil {
			// GraphQL enum is UPPER (IN_REVIEW); domain state is lower_snake.
			s := strings.ToLower(string(*filter.WorkflowState))
			f.WorkflowState = &s
		}
		if filter.Search != nil {
			f.Search = strings.TrimSpace(*filter.Search)
		}
	}
	pages, err := r.CMSSvc.ListPages(ctx, f, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]*model.CMSPage, len(pages))
	for i, p := range pages {
		out[i] = toCMSPageModel(p)
	}
	return out, r.visiblePageListings(ctx, out...)
}
