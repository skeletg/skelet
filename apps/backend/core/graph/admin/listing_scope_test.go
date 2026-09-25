package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestListingChangeNeedsEveryListedChannel(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	onlyA := func() context.Context {
		return adminWithCachedGrants(auth.Grant{Slug: "cms.manage", ChannelID: &a})
	}
	both := func() context.Context {
		return adminWithCachedGrants(auth.Grant{Slug: "cms.manage", ChannelID: &a}, auth.Grant{Slug: "cms.manage", ChannelID: &b})
	}
	global := func() context.Context { return adminWithCachedGrants(auth.Grant{Slug: "cms.manage"}) }
	cases := []struct {
		name     string
		ctx      func() context.Context
		target   uuid.UUID
		listed   []uuid.UUID
		removing bool
		allowed  bool
	}{
		{"first listing on own channel", onlyA, a, nil, false, true},
		{"updating own listing on a shared row", onlyA, a, []uuid.UUID{a, b}, false, true},
		{"listing onto another channel's row", onlyA, a, []uuid.UUID{b}, false, false},
		{"listing on a channel not held", onlyA, b, nil, false, false},
		{"removing own listing from a shared row", onlyA, a, []uuid.UUID{a, b}, true, false},
		{"removing the only listing", onlyA, a, []uuid.UUID{a}, true, true},
		{"covering both channels", both, a, []uuid.UUID{b}, false, true},
		{"global grant", global, a, []uuid.UUID{b}, true, true},
	}
	for _, c := range cases {
		err := requireListingChange(c.ctx(), (*admin.Service)(nil), "cms.manage", c.target, c.listed, c.removing)
		if (err == nil) != c.allowed {
			t.Errorf("%s: err = %v, want allowed=%v", c.name, err, c.allowed)
		}
	}
}

func TestAChannelAdminCannotListOntoAnotherChannelsContent(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	mine, theirs := uuid.New(), uuid.New()
	for _, ch := range []uuid.UUID{mine, theirs} {
		if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'Listing','USD',true)`, ch, "lst-"+ch.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	postID, pageID, catID, typeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	sfx := postID.String()[:8]
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO blog_posts (id, slug) VALUES ($1, $2)`, []any{postID, "post-" + sfx}},
		{`INSERT INTO blog_post_channel_listings (post_id, channel_id) VALUES ($1, $2)`, []any{postID, theirs}},
		{`INSERT INTO cms_page_types (id, slug) VALUES ($1, $2)`, []any{typeID, "type-" + sfx}},
		{`INSERT INTO cms_pages (id, page_type_id, slug, url_path, base_path) VALUES ($1, $2, $3, $3, '')`, []any{pageID, typeID, "page-" + sfx}},
		{`INSERT INTO cms_page_channel_listings (page_id, channel_id) VALUES ($1, $2)`, []any{pageID, theirs}},
		{`INSERT INTO catalog_categories (id, slug, url_path) VALUES ($1, $2, $2)`, []any{catID, "cat-" + sfx}},
		{`INSERT INTO catalog_category_channel_listings (category_id, channel_id) VALUES ($1, $2)`, []any{catID, theirs}},
	} {
		if _, err := conn.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	queries, tx := db.New(conn), txrunner.New(conn)
	r := &mutationResolver{&Resolver{
		BlogSvc:    blog.NewService(blog.Deps{Repo: blog.NewRepository(queries, tx), Tx: tx}),
		CMSSvc:     cms.NewService(cms.Deps{Repo: cms.NewRepository(queries, tx), Tx: tx}),
		CatalogSvc: catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(queries, tx), Tx: tx}),
	}}
	scoped := adminWithCachedGrants(
		auth.Grant{Slug: "blog.manage", ChannelID: &mine},
		auth.Grant{Slug: "cms.manage", ChannelID: &mine},
		auth.Grant{Slug: "catalog.manage", ChannelID: &mine},
	)
	if _, err := r.SetBlogPostChannelListing(scoped, model.SetBlogPostChannelListingInput{PostID: postID.String(), ChannelID: mine.String()}); err == nil {
		t.Error("a channel admin listed another channel's blog post onto their own")
	}
	if _, err := r.SetCMSPageChannelListing(scoped, model.SetCMSPageChannelListingInput{PageID: pageID.String(), ChannelID: mine.String()}); err == nil {
		t.Error("a channel admin listed another channel's CMS page onto their own")
	}
	if _, err := r.SetCategoryListing(scoped, model.SetCategoryListingInput{CategoryID: catID.String(), ChannelID: mine.String()}); err == nil {
		t.Error("a channel admin listed another channel's category onto their own")
	}
	for table, id := range map[string]uuid.UUID{
		"blog_post_channel_listings WHERE post_id":            postID,
		"cms_page_channel_listings WHERE page_id":             pageID,
		"catalog_category_channel_listings WHERE category_id": catID,
	} {
		var n int
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM `+table+` = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("%s has %d listings, want only the original one", table, n)
		}
	}
}
