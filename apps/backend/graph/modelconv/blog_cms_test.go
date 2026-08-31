package modelconv

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/blog"
	"github.com/skeletg/skelet/apps/backend/internal/cms"
)

func TestBlogPostStatusMapsEveryStateAndDefaultsToDraft(t *testing.T) { // an unknown status must surface as DRAFT - the invisible state - never as published
	for in, want := range map[blog.PostStatus]model.BlogPostStatus{
		blog.PostStatusDraft:     model.BlogPostStatusDraft,
		blog.PostStatusScheduled: model.BlogPostStatusScheduled,
		blog.PostStatusPublished: model.BlogPostStatusPublished,
		blog.PostStatusArchived:  model.BlogPostStatusArchived,
		blog.PostStatus("weird"): model.BlogPostStatusDraft,
		blog.PostStatus(""):      model.BlogPostStatusDraft,
	} {
		if got := BlogPostStatus(in); got != want {
			t.Errorf("%q -> %s, want %s", in, got, want)
		}
	}
}

func TestPostListedAndPublishedIsPerChannel(t *testing.T) { // this is the storefront visibility gate - a wrong answer either hides a live post or leaks a draft listing cross-channel
	mine, other := uuid.New(), uuid.New()

	post := func(listings ...blog.PostChannelListing) blog.Post {
		return blog.Post{ID: uuid.New(), Status: blog.PostStatusPublished, ChannelListings: listings}
	}
	if !PostListedAndPublished(post(blog.PostChannelListing{ChannelID: mine, IsPublished: true}), mine) {
		t.Error("a published listing on the right channel = false")
	}
	if PostListedAndPublished(post(blog.PostChannelListing{ChannelID: mine, IsPublished: false}), mine) {
		t.Error("an unpublished listing = true, so a hidden post would render")
	}
	if PostListedAndPublished(post(blog.PostChannelListing{ChannelID: other, IsPublished: true}), mine) {
		t.Error("another channel's listing satisfied this channel's gate")
	}
	if PostListedAndPublished(post(), mine) {
		t.Error("no listings at all = true")
	}
	// several listings - only the matching row decides
	p := post(
		blog.PostChannelListing{ChannelID: other, IsPublished: true},
		blog.PostChannelListing{ChannelID: mine, IsPublished: false},
	)
	if PostListedAndPublished(p, mine) {
		t.Error("another channel's published row overrode this channel's unpublished one")
	}
}

func TestBlogMetadataItemsSortedAndNonNull(t *testing.T) { // map order would reshuffle the admin UI on every refetch, and the schema field is non-null
	got := BlogMetadataItems(blog.Metadata{"z": "1", "a": "2", "m": "3"})
	if len(got) != 3 || got[0].Key != "a" || got[1].Key != "m" || got[2].Key != "z" {
		t.Errorf("items = %v, want sorted a m z", got)
	}
	if empty := BlogMetadataItems(nil); empty == nil || len(empty) != 0 {
		t.Errorf("BlogMetadataItems(nil) = %v, want an empty non-nil list", empty)
	}
}

func TestBlogPostModelHandlesTheOptionals(t *testing.T) { // a draft has no publish date, author or content - none of those may panic or invent a value
	now := time.Now()
	bare := BlogPostModel(blog.Post{ID: uuid.New(), Slug: "hello", Status: blog.PostStatusDraft, CreatedAt: now})
	if bare.PublishedAt != nil || bare.Author != nil || bare.Content != nil {
		t.Errorf("draft = %+v, want the optionals nil", bare)
	}
	if bare.Tags == nil || bare.Categories == nil || bare.AllContent == nil || bare.Metadata == nil {
		t.Error("a list field is nil - the schema says non-null lists")
	}

	pub := now.Add(-time.Hour)
	mins := int32(4)
	full := BlogPostModel(blog.Post{
		ID: uuid.New(), Slug: "tires-101", Status: blog.PostStatusPublished,
		PublishedAt: &pub, ReadingTimeMinutes: &mins,
		Author:  &blog.Author{ID: uuid.New(), Slug: "ana", DisplayName: "Ana"},
		Content: &blog.PostContent{Locale: "en", Title: "Tires 101", Body: json.RawMessage(`{"blocks":[]}`)},
	})
	if full.PublishedAt == nil || !full.PublishedAt.Equal(pub) {
		t.Errorf("PublishedAt = %v, want %s", full.PublishedAt, pub)
	}
	if full.Author == nil || full.Author.DisplayName != "Ana" {
		t.Errorf("Author = %+v, want Ana", full.Author)
	}
	if full.Content == nil || full.Content.Title != "Tires 101" {
		t.Errorf("Content = %+v, want the resolved locale content", full.Content)
	}
	if full.ReadingTimeMinutes == nil || *full.ReadingTimeMinutes != 4 {
		t.Errorf("ReadingTimeMinutes = %v, want 4", full.ReadingTimeMinutes)
	}
}

func TestUnmarshalJSONFailsSoftToNull(t *testing.T) { // block content is stored jsonb and rendered by the builder - a corrupt blob must read as null, not fail the whole post query
	for name, raw := range map[string]json.RawMessage{
		"nil":       nil,
		"empty":     {},
		"corrupt":   json.RawMessage(`{"blocks":`),
		"bare word": json.RawMessage(`blocks`),
	} {
		if got := unmarshalJSON(raw); got != nil {
			t.Errorf("%s: unmarshalJSON = %v, want nil", name, got)
		}
	}
	got := unmarshalJSON(json.RawMessage(`{"blocks":[{"type":"hero"}]}`))
	m, ok := got.(map[string]any)
	if !ok || m["blocks"] == nil {
		t.Errorf("a valid blob = %#v, want the decoded object", got)
	}
	if got := unmarshalJSON(json.RawMessage(`null`)); got != nil {
		t.Errorf("json null = %v, want nil", got)
	}
}

func TestCMSRelationshipTargetMapping(t *testing.T) { // relationship rows drive which picker the dashboard opens - a wrong default would open the page picker for a product link
	for in, want := range map[cms.RelationshipTarget]model.CMSRelationshipTarget{
		cms.TargetBlogPost:   model.CMSRelationshipTargetBlogPost,
		cms.TargetProduct:    model.CMSRelationshipTargetProduct,
		cms.TargetCollection: model.CMSRelationshipTargetCollection,
		cms.TargetCategory:   model.CMSRelationshipTargetCategory,
		cms.TargetCMSPage:    model.CMSRelationshipTargetCmsPage,
	} {
		if got := CMSRelationshipTarget(in); got != want {
			t.Errorf("%q -> %s, want %s", in, got, want)
		}
	}
}

func TestBlogPostChannelListingsKeepTheirFlags(t *testing.T) { // SEONoindex per listing is what keeps a staging channel's copy out of Google while the main one indexes
	ch := uuid.New()
	got := BlogPostChannelListings([]blog.PostChannelListing{
		{ChannelID: ch, IsPublished: true, SEONoindex: true},
	})
	if len(got) != 1 {
		t.Fatalf("%d listings, want 1", len(got))
	}
	if got[0].ChannelID != ch.String() || !got[0].IsPublished || !got[0].SeoNoindex {
		t.Errorf("listing = %+v, want the flags carried through", got[0])
	}
}
