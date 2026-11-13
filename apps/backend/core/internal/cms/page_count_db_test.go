package cms

import (
	"context"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// cmsPageCount has to count what the list returns: through a channel's listings, and through the index for a search
func TestCountPagesMatchesTheListForEveryFilter(t *testing.T) {
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
	locale := "pc" + sfx[:6]
	exec(`INSERT INTO locales (code, name) VALUES ($1, $1)`, locale)
	channel := one(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, $1, 'USD') RETURNING id`, "pc-"+sfx)
	pageType := one(`INSERT INTO cms_page_types (slug) VALUES ($1) RETURNING id`, "pc-"+sfx)
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM cms_pages WHERE page_type_id = $1`, pageType)
		_, _ = conn.Exec(`DELETE FROM cms_page_types WHERE id = $1`, pageType)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, channel)
		_, _ = conn.Exec(`DELETE FROM locales WHERE code = $1`, locale)
	})
	page := func(name string, published bool, workflow string, parent *uuid.UUID) uuid.UUID {
		slug := "pc-" + sfx + "-" + name
		return one(`INSERT INTO cms_pages (page_type_id, parent_id, slug, url_path, base_path, is_published, workflow_state) VALUES ($1, $2, $3, $3, $3, $4, $5) RETURNING id`,
			pageType, parent, slug, published, workflow)
	}
	live := page("live", true, "published", nil)
	page("review", false, "in_review", nil)
	child := page("child", true, "published", &live)
	exec(`INSERT INTO cms_page_channel_listings (page_id, channel_id, is_published) VALUES ($1, $3, TRUE), ($2, $3, FALSE)`, live, child, channel)
	for id, text := range map[uuid.UUID]string{live: "alpha zebra", child: "beta zebra"} {
		exec(`INSERT INTO cms_page_content (page_id, locale, title, search_vector) VALUES ($1, $2, $3, to_tsvector('simple', $3))`, id, locale, text)
	}

	repo := NewRepository(db.New(conn), nil)
	inReview := "in_review"
	own := func(f PageListFilter) PageListFilter {
		f.PageTypeID = &pageType
		return f
	}
	cases := []struct {
		name   string
		filter PageListFilter
		want   int64
	}{
		{"every state", own(PageListFilter{}), 3},
		{"published", own(PageListFilter{OnlyPublished: true}), 2},
		{"one workflow state", own(PageListFilter{WorkflowState: &inReview}), 1},
		{"under a parent", own(PageListFilter{ParentID: &live}), 1},
		{"on a store", PageListFilter{ChannelID: &channel}, 1},
		{"on a store, by type", own(PageListFilter{ChannelID: &channel}), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			listed := c.filter
			listed.Limit = MaxPageListLimit
			pages, err := repo.ListPages(ctx, listed)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			got, err := repo.CountPages(ctx, c.filter)
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			if got != int64(len(pages)) || got != c.want {
				t.Errorf("counted %d, the list holds %d, want %d", got, len(pages), c.want)
			}
		})
	}

	t.Run("a search", func(t *testing.T) {
		for query, want := range map[string]int64{"zebra": 2, "alpha": 1, "nothing-like-it": 0} {
			filter := own(PageListFilter{Limit: MaxPageListLimit})
			pages, err := repo.ListPagesSearch(ctx, filter, locale, query)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			got, err := repo.CountPagesSearch(ctx, filter, locale, query)
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			if got != int64(len(pages)) || got != want {
				t.Errorf("%q: counted %d, the search holds %d, want %d", query, got, len(pages), want)
			}
		}
	})
}
