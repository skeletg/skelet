package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Same versioning rationale as ProductIndex - bump the suffix on an incompatible mapping change, then flip an alias
const BlogPostIndex = "blog_posts_v1"

// is_published folds the post's global status AND the per-store listing flag - a false on either still indexes, so admin tools can search drafts later, but the front resolver filters on it
type BlogPostDoc struct {
	PostID      uuid.UUID  `json:"post_id"`
	ChannelID   uuid.UUID  `json:"channel_id"`
	Locale      string     `json:"locale"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Excerpt     string     `json:"excerpt,omitempty"`
	Body        string     `json:"body,omitempty"`
	AuthorName  string     `json:"author_name,omitempty"`
	IsPublished bool       `json:"is_published"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	IndexedAt   time.Time  `json:"indexed_at"`
}

// Built from the live config registry for dashboard-driven retuning - title keeps a .keyword subfield for admin sorts, excerpt and body are full-text only
func buildBlogPostMapping(ctx context.Context, cfg *config.Service) (string, error) {
	shards := cfgIntOr(ctx, cfg, pathMappingShards, 1)
	replicas := cfgIntOr(ctx, cfg, pathMappingReplicas, 0)
	analyzer := cfgStringOr(ctx, cfg, pathMappingAnalyzer, analyzerStandard)
	minGram := cfgIntOr(ctx, cfg, pathMappingEdgeNgramMin, 2)
	maxGram := cfgIntOr(ctx, cfg, pathMappingEdgeNgramMax, 15)

	settings := map[string]any{
		"index": map[string]any{
			"number_of_shards":   shards,
			"number_of_replicas": replicas,
		},
	}

	titleField := map[string]any{
		"type": "text",
		"fields": map[string]any{
			"keyword": map[string]any{"type": "keyword", "ignore_above": 256},
		},
	}
	excerptField := map[string]any{"type": "text"}
	bodyField := map[string]any{"type": "text"}

	if analyzer == analyzerEdgeNgram {
		settings["analysis"] = edgeNgramAnalysisSettings(minGram, maxGram)
		for _, f := range []map[string]any{titleField, excerptField, bodyField} {
			f["analyzer"] = "edge_ngram_analyzer"
			f["search_analyzer"] = "standard"
		}
	}

	body := map[string]any{
		"settings": settings,
		"mappings": map[string]any{
			"properties": map[string]any{
				"post_id":      map[string]any{"type": "keyword"},
				"channel_id":   map[string]any{"type": "keyword"},
				"locale":       map[string]any{"type": "keyword"},
				"slug":         map[string]any{"type": "keyword"},
				"title":        titleField,
				"excerpt":      excerptField,
				"body":         bodyField,
				"author_name":  map[string]any{"type": "keyword"},
				"is_published": map[string]any{"type": "boolean"},
				"published_at": map[string]any{"type": "date"},
				"indexed_at":   map[string]any{"type": "date"},
			},
		},
	}
	out, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal blog post mapping: %w", err)
	}
	return string(out), nil
}

type BlogPostSource interface {
	GetPost(ctx context.Context, id uuid.UUID, locale string) (blog.Post, error)
	ListPosts(ctx context.Context, filter blog.PostListFilter, locale string) ([]blog.Post, error)
}

type BlogPostIndexer struct {
	client  *Client
	blog    BlogPostSource
	channel ChannelSource
	cfg     *config.Service
}

func NewBlogPostIndexer(c *Client, b BlogPostSource, sv ChannelSource, cfg *config.Service) *BlogPostIndexer {
	return &BlogPostIndexer{client: c, blog: b, channel: sv, cfg: cfg}
}

func (i *BlogPostIndexer) EnsureIndex(ctx context.Context) error {
	mapping, err := buildBlogPostMapping(ctx, i.cfg)
	if err != nil {
		return err
	}
	return i.client.EnsureAliasedIndex(ctx, BlogPostIndex, mapping)
}

// RebuildAndSwap mirrors the product indexer's zero-downtime rebuild - blog search
// keeps answering from the old docs until the atomic alias flip
func (i *BlogPostIndexer) RebuildAndSwap(ctx context.Context) (int, error) {
	mapping, err := buildBlogPostMapping(ctx, i.cfg)
	if err != nil {
		return 0, err
	}
	return rebuildAndSwap(ctx, i.client, BlogPostIndex, mapping, i.reindexAllInto)
}

func (i *BlogPostIndexer) Recreate(ctx context.Context) error {
	mapping, err := buildBlogPostMapping(ctx, i.cfg)
	if err != nil {
		return err
	}
	return i.client.RecreateIndex(ctx, BlogPostIndex, mapping)
}

// Locales come from each channel listing the post, and the post carries AllContent keyed by locale, so build time picks from that rather than a resolution roundtrip per (store, locale)
// New docs and stale-doc deletes go in one _bulk - the delete-first order this replaced left a post with ZERO docs on any write error
func (i *BlogPostIndexer) Upsert(ctx context.Context, postID uuid.UUID) error {
	post, err := i.blog.GetPost(ctx, postID, "")
	if err != nil {
		return fmt.Errorf("load post: %w", err)
	}
	channels, err := i.channelsByID(ctx)
	if err != nil {
		return err
	}
	docs := i.postDocs(post, channels, false)
	if len(docs) == 0 {
		return i.Delete(ctx, postID)
	}
	existing, staleErr := i.client.ListDocIDsByField(ctx, BlogPostIndex, "post_id", postID.String())
	fresh := make(map[string]struct{}, len(docs))
	for _, d := range docs {
		fresh[d.ID] = struct{}{}
	}
	for _, id := range existing {
		if _, ok := fresh[id]; !ok {
			docs = append(docs, BulkDoc{ID: id, Delete: true})
		}
	}
	if err := i.client.Bulk(ctx, BlogPostIndex, docs); err != nil {
		return err
	}
	if staleErr != nil {
		return fmt.Errorf("stale docs left in place: %w", staleErr)
	}
	return nil
}

func (i *BlogPostIndexer) channelsByID(ctx context.Context) (map[uuid.UUID]channel.Channel, error) {
	channels, err := i.channel.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load channels: %w", err)
	}
	out := make(map[uuid.UUID]channel.Channel, len(channels))
	for _, sv := range channels {
		out[sv.ID] = sv
	}
	return out, nil
}

// One doc per (store, locale) the post's listings span - a listing on a deleted store is skipped
func (i *BlogPostIndexer) postDocs(post blog.Post, channels map[uuid.UUID]channel.Channel, ifAbsent bool) []BulkDoc {
	contentByLocale := make(map[string]blog.PostContent, len(post.AllContent))
	for _, c := range post.AllContent {
		contentByLocale[c.Locale] = c
	}
	authorName := ""
	if post.Author != nil {
		// Whichever localized name comes back first - the doc's locale and the author display are independent
		if post.Author.Content != nil {
			authorName = post.Author.Content.Name
		} else if len(post.Author.AllContent) > 0 {
			authorName = post.Author.AllContent[0].Name
		}
	}
	var docs []BulkDoc
	for _, lst := range post.ChannelListings {
		sv, ok := channels[lst.ChannelID]
		if !ok {
			continue
		}
		for _, locale := range localeCodes(sv) {
			docs = append(docs, BulkDoc{
				ID:       docID(post.ID, sv.ID, locale),
				Doc:      i.buildDoc(post, lst, sv, locale, contentByLocale, authorName),
				IfAbsent: ifAbsent,
			})
		}
	}
	return docs
}

func (i *BlogPostIndexer) Delete(ctx context.Context, postID uuid.UUID) error {
	return i.client.DeleteByField(ctx, BlogPostIndex, "post_id", postID.String())
}

type BlogPostHit struct {
	PostID      uuid.UUID
	Slug        string
	Title       string
	Excerpt     string
	PublishedAt *time.Time
	AuthorName  string
}

// Scoped to the (store, locale) pair and is_published, which already folds the global status and the per-store listing flag at index time
func (i *BlogPostIndexer) SearchBlogPosts(ctx context.Context, query string, channelID uuid.UUID, locale string, limit, offset int) ([]BlogPostHit, int64, error) {
	limitDefault := cfgIntOr(ctx, i.cfg, pathLimitDefault, 20)
	limitMax := cfgIntOr(ctx, i.cfg, pathLimitMax, 100)
	if limit <= 0 {
		limit = int(limitDefault)
	} else if int64(limit) > limitMax {
		limit = int(limitMax)
	}
	if offset < 0 {
		offset = 0
	}
	q := strings.TrimSpace(query)
	// Mirrors product search - short and empty queries return nothing, with an admin-configurable minimum length
	if utf8.RuneCountInString(q) < int(cfgIntOr(ctx, i.cfg, pathMinQueryLength, 1)) {
		return []BlogPostHit{}, 0, nil
	}
	attrs := loadAttributes(ctx, i.cfg, pathBlogAttributes, defaultBlogAttributes)
	body := map[string]any{
		"from":    offset,
		"size":    limit,
		"_source": []string{"post_id", "slug", "title", "excerpt", "published_at", "author_name"},
		"query": map[string]any{
			"bool": map[string]any{
				"must": []any{buildSearchQuery(q, attrs)},
				"filter": []any{
					map[string]any{"term": map[string]any{"channel_id": channelID.String()}},
					map[string]any{"term": map[string]any{"locale": locale}},
					map[string]any{"term": map[string]any{"is_published": true}},
				},
			},
		},
	}
	hits := make([]BlogPostHit, 0, limit)
	total, err := i.client.Search(ctx, BlogPostIndex, body, func(raw json.RawMessage) error {
		var src BlogPostDoc
		if err := json.Unmarshal(raw, &src); err != nil {
			return err
		}
		hits = append(hits, BlogPostHit{
			PostID:      src.PostID,
			Slug:        src.Slug,
			Title:       src.Title,
			Excerpt:     src.Excerpt,
			PublishedAt: src.PublishedAt,
			AuthorName:  src.AuthorName,
		})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return hits, total, nil
}

// Keeps going past a single bad post and reports how far it got - aborting on the first error and returning 0 hid
// whether 10 or 10,000 posts had made it, exactly the failure ProductIndexer.ReindexAll was fixed to stop
func (i *BlogPostIndexer) ReindexAll(ctx context.Context) (int, error) {
	return i.reindexAllInto(ctx, BlogPostIndex)
}

func (i *BlogPostIndexer) reindexAllInto(ctx context.Context, target string) (int, error) {
	posts, err := i.blog.ListPosts(ctx, blog.PostListFilter{}, "")
	if err != nil {
		return 0, fmt.Errorf("list posts: %w", err)
	}
	channels, err := i.channelsByID(ctx)
	if err != nil {
		return 0, err
	}
	done, failed := 0, 0
	var firstErr error
	fail := func(n int, err error) {
		failed += n
		if firstErr == nil {
			firstErr = err
		}
	}
	for start := 0; start < len(posts); start += reindexPageSize {
		if ctx.Err() != nil {
			return done, fmt.Errorf("reindex stopped after %d/%d posts: %w", done, len(posts), ctx.Err())
		}
		var docs []BulkDoc
		built := 0
		for _, p := range posts[start:min(start+reindexPageSize, len(posts))] {
			post, err := i.blog.GetPost(ctx, p.ID, "")
			if err != nil {
				fail(1, fmt.Errorf("load %s: %w", p.ID, err))
				continue
			}
			docs = append(docs, i.postDocs(post, channels, target != BlogPostIndex)...)
			built++
		}
		if err := i.client.Bulk(ctx, target, docs); err != nil {
			fail(built, err)
			continue
		}
		done += built
	}
	if firstErr != nil {
		return done, fmt.Errorf("reindexed %d/%d posts, %d failed - first error: %w", done, len(posts), failed, firstErr)
	}
	return done, nil
}

func (i *BlogPostIndexer) buildDoc(
	post blog.Post,
	listing blog.PostChannelListing,
	sv channel.Channel,
	locale string,
	contentByLocale map[string]blog.PostContent,
	authorName string,
) BlogPostDoc {
	// Locale fallback chain: requested, store default, first available
	content, ok := contentByLocale[locale]
	if !ok {
		content, ok = contentByLocale[sv.DefaultLocale]
	}
	if !ok && len(post.AllContent) > 0 {
		content = post.AllContent[0]
	}
	// Stamp the combined "globally published AND per-store listed" flag here so the query layer filters on one field
	combinedPublished := listing.IsPublished && post.Status == blog.PostStatusPublished
	return BlogPostDoc{
		PostID:      post.ID,
		ChannelID:   sv.ID,
		Locale:      locale,
		Slug:        post.Slug,
		Title:       content.Title,
		Excerpt:     stripJSON(content.Excerpt),
		Body:        stripJSON(content.Body),
		AuthorName:  authorName,
		IsPublished: combinedPublished,
		PublishedAt: post.PublishedAt,
		IndexedAt:   time.Now().UTC(),
	}
}
