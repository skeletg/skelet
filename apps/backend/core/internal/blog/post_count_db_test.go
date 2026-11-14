package blog

import (
	"context"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// One count query stands in for eight list queries, so each branch the list can take is held to the list's own answer
func TestCountPostsMatchesTheListForEveryFilter(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	sfx := uuid.NewString()[:8]
	one := func(query string, args ...any) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := conn.QueryRow(query, args...).Scan(&id); err != nil {
			t.Fatalf("seed: %v\n%s", err, query)
		}
		return id
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, query)
		}
	}
	channel := one(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, $1, 'USD') RETURNING id`, "cnt-"+sfx)
	author := one(`INSERT INTO blog_authors (slug, display_name) VALUES ($1, 'Counter') RETURNING id`, "cnt-"+sfx)
	category := one(`INSERT INTO blog_categories (slug, url_path) VALUES ($1, $1) RETURNING id`, "cnt-"+sfx)
	tag := one(`INSERT INTO blog_tags (slug) VALUES ($1) RETURNING id`, "cnt-"+sfx)
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM blog_posts WHERE slug LIKE $1`, "cnt-"+sfx+"-%")
		_, _ = conn.Exec(`DELETE FROM blog_tags WHERE id = $1`, tag)
		_, _ = conn.Exec(`DELETE FROM blog_categories WHERE id = $1`, category)
		_, _ = conn.Exec(`DELETE FROM blog_authors WHERE id = $1`, author)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, channel)
	})
	post := func(name, status string, by *uuid.UUID) uuid.UUID {
		return one(`INSERT INTO blog_posts (slug, status, author_id, published_at) VALUES ($1, $2, $3, CASE WHEN $2 IN ('published', 'scheduled') THEN NOW() + INTERVAL '1 day' END) RETURNING id`, "cnt-"+sfx+"-"+name, status, by)
	}
	live := post("live", "published", &author)
	unlisted := post("unlisted", "published", nil)
	draft := post("draft", "draft", &author)
	post("scheduled", "scheduled", nil)
	archived := post("archived", "archived", nil)
	for _, id := range []uuid.UUID{live, draft} {
		exec(`INSERT INTO blog_post_categories (post_id, category_id) VALUES ($1, $2)`, id, category)
	}
	for _, id := range []uuid.UUID{live, draft, archived} {
		exec(`INSERT INTO blog_post_tags (post_id, tag_id) VALUES ($1, $2)`, id, tag)
	}
	// A draft listed on the store, and a published post whose listing is switched off: neither is on the store
	exec(`INSERT INTO blog_post_channel_listings (post_id, channel_id, is_published) VALUES ($1, $3, TRUE), ($2, $3, TRUE)`, live, draft, channel)
	exec(`INSERT INTO blog_post_channel_listings (post_id, channel_id, is_published) VALUES ($1, $2, FALSE)`, unlisted, channel)

	repo := NewRepository(db.New(conn), nil)
	draftStatus := PostStatusDraft
	cases := []struct {
		name   string
		filter PostListFilter
		want   int64 // -1 where other tests' rows share the filter, and only the list can say
	}{
		{"everything", PostListFilter{}, -1},
		{"one status", PostListFilter{Status: &draftStatus}, -1},
		{"published", PostListFilter{OnlyPublished: true}, -1},
		{"category", PostListFilter{CategoryID: &category}, 2},
		{"category, published", PostListFilter{CategoryID: &category, OnlyPublished: true}, 1},
		{"tag", PostListFilter{TagID: &tag}, 3},
		{"tag, published", PostListFilter{TagID: &tag, OnlyPublished: true}, 1},
		{"author", PostListFilter{AuthorID: &author}, 2},
		{"author, published", PostListFilter{AuthorID: &author, OnlyPublished: true}, 1},
		{"store", PostListFilter{ChannelID: &channel}, 1},
		{"store and category", PostListFilter{ChannelID: &channel, CategoryID: &category}, 1},
		{"store and tag", PostListFilter{ChannelID: &channel, TagID: &tag}, 1},
		{"store and author", PostListFilter{ChannelID: &channel, AuthorID: &author}, 1},
		{"category wins over tag", PostListFilter{CategoryID: &category, TagID: &tag}, 2},
		{"a status is ignored beside a category", PostListFilter{CategoryID: &category, Status: &draftStatus}, 2},
		{"a status is ignored on a store", PostListFilter{ChannelID: &channel, Status: &draftStatus}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			listed := c.filter
			listed.Limit = 100000
			posts, err := repo.ListPosts(ctx, listed)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			got, err := repo.CountPosts(ctx, c.filter)
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			if got != int64(len(posts)) {
				t.Errorf("counted %d, the list holds %d", got, len(posts))
			}
			if c.want >= 0 && got != c.want {
				t.Errorf("counted %d, want %d", got, c.want)
			}
		})
	}
}
