package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/internal/blog"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/cms"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	dbconn "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/locale"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
)

var URLRewriteCmd = &cobra.Command{
	Use:   "urlrewrite",
	Short: "URL rewrite (canonical-URL) maintenance",
}

var urlRewriteBackfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Upsert canonical url_rewrites for every product, collection, blog post, and CMS page that doesn't already have one",
	Long: `Reconciles the url_rewrites table with the catalog/blog/CMS tables by
walking every entity and calling urlrewrite.SyncCanonical for any row
that doesn't already own a canonical (redirect_type=0) rewrite.

Idempotent: re-running is a no-op once every entity has a row. Safe to
re-run after a seed or after fixing a service that wasn't syncing.

Canonical-path rules per entity:
  product     COALESCE(url_path, slug)
  collection  slug
  blog post   "blog/" + slug
  CMS page    url_path
`,
	RunE: runURLRewriteBackfill,
}

func init() {
	URLRewriteCmd.AddCommand(urlRewriteBackfillCmd)
}

type urlRewriteServices struct {
	catalog    *catalog.Service
	blog       *blog.Service
	cms        *cms.Service
	urlrewrite *urlrewrite.Service
	cleanup    func()
}

func newURLRewriteServices() (*urlRewriteServices, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	queries := db.New(conn)
	txMgr := tx.New(conn)
	configSvc := config.NewService(config.NewRepository(queries, txMgr))
	localeSvc := locale.NewService(locale.NewRepository(queries))
	channelSvc := channel.NewService(channel.NewRepository(queries, txMgr), configSvc)
	channelSvc.SetLocaleValidator(localeSvc)
	catalogSvc := catalog.NewService(catalog.Deps{
		Repo:    catalog.NewRepository(queries, txMgr),
		Tx:      txMgr,
		Channel: channelSvc,
		Locales: localeSvc,
	})
	blogSvc := blog.NewService(blog.Deps{
		Repo:    blog.NewRepository(queries, txMgr),
		Tx:      txMgr,
		Locales: localeSvc,
	})
	cmsSvc := cms.NewService(cms.Deps{
		Repo:    cms.NewRepository(queries, txMgr),
		Tx:      txMgr,
		Locales: localeSvc,
	})
	urlRewriteSvc := urlrewrite.NewService(urlrewrite.NewRepository(queries))
	return &urlRewriteServices{
		catalog:    catalogSvc,
		blog:       blogSvc,
		cms:        cmsSvc,
		urlrewrite: urlRewriteSvc,
		cleanup:    func() { conn.Close() },
	}, nil
}

func runURLRewriteBackfill(cmd *cobra.Command, _ []string) error {
	svcs, err := newURLRewriteServices()
	if err != nil {
		return err
	}
	defer svcs.cleanup()
	ctx := context.Background()
	totals := map[string]int{}
	products, err := svcs.catalog.ListProducts(ctx, "", 0) // 0 = every row - a capped backfill leaves products without a canonical URL
	if err != nil {
		return fmt.Errorf("list products: %w", err)
	}
	for _, p := range products {
		path := p.URL
		if err := svcs.urlrewrite.SyncCanonical(ctx, string(urlrewrite.EntityTypeProduct), p.ID, path); err != nil {
			return fmt.Errorf("sync product %s: %w", p.ID, err)
		}
		totals["product"]++
	}
	collections, err := svcs.catalog.ListCollections(ctx, "")
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	for _, c := range collections {
		if err := svcs.urlrewrite.SyncCanonical(ctx, string(urlrewrite.EntityTypeCollection), c.ID, c.Slug); err != nil {
			return fmt.Errorf("sync collection %s: %w", c.ID, err)
		}
		totals["collection"]++
	}
	posts, err := svcs.blog.ListPosts(ctx, blog.PostListFilter{}, "")
	if err != nil {
		return fmt.Errorf("list posts: %w", err)
	}
	for _, p := range posts {
		if err := svcs.urlrewrite.SyncCanonical(ctx, string(urlrewrite.EntityTypeBlogPost), p.ID, "blog/"+p.Slug); err != nil {
			return fmt.Errorf("sync blog post %s: %w", p.ID, err)
		}
		totals["blog_post"]++
	}
	pages, err := svcs.cms.ListPages(ctx, cms.PageListFilter{}, "")
	if err != nil {
		return fmt.Errorf("list cms pages: %w", err)
	}
	for _, page := range pages {
		if page.URLPath == "" { // shouldn't happen, but the urlrewrite write rejects an empty path and would abort the whole backfill
			continue
		}
		if err := svcs.urlrewrite.SyncCanonical(ctx, string(urlrewrite.EntityTypeCMSPage), page.ID, page.URLPath); err != nil {
			return fmt.Errorf("sync cms page %s: %w", page.ID, err)
		}
		totals["cms_page"]++
	}
	for kind, n := range totals {
		fmt.Printf("synced %d %s\n", n, kind)
	}
	return nil
}
