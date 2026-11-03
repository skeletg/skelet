package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type cliFlatModeToggle struct{ cfg *config.Service } // a CLI-built inventory service has no config wiring of its own, and without the flag its salable maths diverges from the server's

func (t cliFlatModeToggle) FlatModeEnabled(ctx context.Context) bool {
	v, err := t.cfg.GetBool(ctx, "sales/inventory/flat_mode", config.DefaultScope)
	return err == nil && v
}

// Deliberately slim - the indexer only calls SalableByProductIDs, to stamp is_in_stock during a reindex
func newReindexInventory(queries *db.Queries, txMgr *txrunner.Manager, configSvc *config.Service) *inventory.Service {
	inv := inventory.NewService(inventory.NewRepository(queries, txMgr), txMgr)
	inv.SetFlatModeToggle(cliFlatModeToggle{cfg: configSvc})
	return inv
}

var SearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search index maintenance (products, blog posts)",
}

var searchReindexCmd = &cobra.Command{
	Use:   "reindex <target>",
	Short: "Drop + recreate an index and bulk-index every row from Postgres",
	Long: `Targets:
  products  reindex only the product index
  blog      reindex only the blog post index
  all       both

Core indexes rebuild into a fresh physical index and flip an alias
atomically, so the store keeps serving the old docs for the whole run -
use it when the mapping shape changes or when you suspect drift between
Postgres and OpenSearch. Live mutations during the run land in the OLD
index and reconverge through the event subscribers afterwards.`,
	Args: cobra.ExactArgs(1),
	RunE: runSearchReindex,
}

var searchReindexTimeout time.Duration

func init() {
	// Was a hardcoded 5 minutes, which silently capped a completable reindex at whatever fit - a 100k catalogue died partway and left the index holding a fraction of the products, served as if complete
	searchReindexCmd.Flags().DurationVar(&searchReindexTimeout, "timeout", time.Hour,
		"abort the reindex after this long (0 for no limit)")
	SearchCmd.AddCommand(searchReindexCmd)
}

func runSearchReindex(cmd *cobra.Command, args []string) error {
	target := strings.ToLower(args[0])
	svcs, err := newSearchServices()
	if err != nil {
		return err
	}
	defer svcs.cleanup()
	ctx, cancel := context.Background(), context.CancelFunc(func() {})
	if searchReindexTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, searchReindexTimeout)
	}
	defer cancel()
	// The CLI wires its services by hand with no plugin host, so only the CORE indexes exist here - a plugin-registered
	// index reindexes through the search.reindex admin action, which runs inside the server where the plugin is installed
	results, err := svcs.indexes.ReindexTargets(ctx, []string{target})
	if err != nil {
		return err
	}
	var firstErr error
	for _, res := range results {
		switch {
		case res.Skipped:
			fmt.Printf("%s: recreate failed: %v\n", res.Slug, res.Err)
		case res.Err != nil:
			fmt.Printf("%s: reindexed %d, with errors: %v\n", res.Slug, res.Count, res.Err)
		default:
			fmt.Printf("%s: reindexed %d\n", res.Slug, res.Count)
		}
		if res.Err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", res.Slug, res.Err)
		}
	}
	return firstErr
}

type searchServices struct {
	indexes *search.IndexRegistry
	cleanup func()
}

func newSearchServices() (*searchServices, error) {
	osURL := strings.TrimSpace(os.Getenv("OPENSEARCH_URL"))
	if osURL == "" {
		return nil, fmt.Errorf("OPENSEARCH_URL is not set")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	queries := db.New(conn)
	txMgr := txrunner.New(conn)
	configSvc := config.NewService(configstore.New(queries))
	localeSvc := locale.NewService(locale.NewRepository(queries))
	channelSvc := channel.NewService(channel.NewRepository(queries, txMgr))
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
	client, err := search.NewClient([]string{osURL})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("opensearch client: %w", err)
	}
	registry := search.NewIndexRegistry()
	// Same slugs the server registers, so `skelet cli search reindex products` and the
	// admin action address the same thing
	_ = registry.Register(search.RegisteredIndex{Slug: "products", Lifecycle: newRankedProductIndexer(client, catalogSvc, channelSvc, queries, txMgr, configSvc)})
	_ = registry.Register(search.RegisteredIndex{Slug: "blog", Lifecycle: search.NewBlogPostIndexer(client, blogSvc, channelSvc, configSvc)})
	return &searchServices{
		indexes: registry,
		cleanup: func() { conn.Close() },
	}, nil
}

// The ranking source has to be attached here too - a CLI reindex must stamp the same sales_30d signal the server-side outbox upserts do
func newRankedProductIndexer(client *search.Client, catalogSvc *catalog.Service, channelSvc *channel.Service, queries *db.Queries, txMgr *txrunner.Manager, configSvc *config.Service) *search.ProductIndexer {
	idx := search.NewProductIndexer(client, catalogSvc, channelSvc, newReindexInventory(queries, txMgr, configSvc), configSvc)
	idx.SetSalesSource(cliSalesSource{queries: queries})
	return idx
}

type cliSalesSource struct{ queries *db.Queries }

// Satisfies search.BatchSalesSource, which the bulk reindex prefers - one query per page instead of one per product
func (s cliSalesSource) SalesVelocity30dBySKUs(ctx context.Context, skus []string) (map[string]map[uuid.UUID]float64, error) {
	rows, err := s.queries.SalesVelocity30dBySKUs(ctx, skus)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[uuid.UUID]float64, len(rows))
	for _, r := range rows {
		if out[r.ProductSku] == nil {
			out[r.ProductSku] = map[uuid.UUID]float64{}
		}
		out[r.ProductSku][r.ChannelID] = r.Units
	}
	return out, nil
}

func (s cliSalesSource) SalesVelocity30d(ctx context.Context, sku string) (map[uuid.UUID]float64, error) {
	rows, err := s.queries.SalesVelocity30dBySKU(ctx, sku)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]float64, len(rows))
	for _, r := range rows {
		out[r.ChannelID] = r.Units
	}
	return out, nil
}
