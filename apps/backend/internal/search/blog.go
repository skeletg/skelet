package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/blog"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/config"
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
func (i *BlogPostIndexer) Upsert(ctx context.Context, postID uuid.UUID) error {
	return i.indexPostInto(ctx, BlogPostIndex, postID, true)
}

// cleanupStale is skipped by the rebuild - a fresh physical index has no leftovers
// to reconcile, and the per-post _search is a large share of the rebuild cost
func (i *BlogPostIndexer) indexPostInto(ctx context.Context, target string, postID uuid.UUID, cleanupStale bool) error {
	post, err := i.blog.GetPost(ctx, postID, "")
	if err != nil {
		return fmt.Errorf("load post: %w", err)
	}
	if len(post.ChannelListings) == 0 {
		// No store lists it - clear whatever a previous publish state left behind
		return i.client.DeleteByField(ctx, target, "post_id", postID.String())
	}
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
	// Write the new set first, then drop leftovers - the delete-first order this replaced meant any write error left the post with ZERO docs,
	// unfindable until a manual reindex, where a brief overlap of stale siblings is merely cosmetic. Same inversion as ProductIndexer.Upsert
	fresh := map[string]struct{}{}
	for _, lst := range post.ChannelListings {
		sv, err := i.channel.GetByID(ctx, lst.ChannelID)
		if err != nil {
			continue
		}
		for _, locale := range localeCodes(sv) {
			doc := i.buildDoc(post, lst, sv, locale, contentByLocale, authorName)
			id := docID(post.ID, sv.ID, locale)
			if err := i.client.IndexDoc(ctx, target, id, doc); err != nil {
				return err
			}
			fresh[id] = struct{}{}
		}
	}
	if len(fresh) == 0 {
		return i.client.DeleteByField(ctx, target, "post_id", postID.String())
	}
	if !cleanupStale {
		return nil
	}
	stale, err := i.client.ListDocIDsByField(ctx, target, "post_id", postID.String())
	if err != nil {
		log.Printf("[search] list existing docs for post %s: %v", postID, err)
		return nil
	}
	for _, id := range stale {
		if _, ok := fresh[id]; ok {
			continue
		}
		if err := i.client.DeleteDoc(ctx, target, id); err != nil {
			log.Printf("[search] delete stale blog doc %s: %v", id, err)
		}
	}
	return nil
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
	done, failed := 0, 0
	var firstErr error
	for _, p := range posts {
		if ctx.Err() != nil {
			return done, fmt.Errorf("reindex stopped after %d/%d posts: %w", done, len(posts), ctx.Err())
		}
		if err := i.indexPostInto(ctx, target, p.ID, false); err != nil {
			failed++
			if firstErr == nil {
				firstErr = fmt.Errorf("upsert %s: %w", p.ID, err)
			}
			continue
		}
		done++
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
