package wiring

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"

	graphadmin "github.com/skeletg/skelet/apps/backend/graph/admin"
	"github.com/skeletg/skelet/apps/backend/internal/ai"
	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/audit"
	"github.com/skeletg/skelet/apps/backend/internal/backup"
	"github.com/skeletg/skelet/apps/backend/internal/blog"
	"github.com/skeletg/skelet/apps/backend/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/chat"
	"github.com/skeletg/skelet/apps/backend/internal/cms"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/currency"
	"github.com/skeletg/skelet/apps/backend/internal/datajob"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/destination"
	"github.com/skeletg/skelet/apps/backend/internal/flow"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
	"github.com/skeletg/skelet/apps/backend/internal/linkgraph"
	"github.com/skeletg/skelet/apps/backend/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/internal/notification"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	paymentpkg "github.com/skeletg/skelet/apps/backend/internal/payment"
	pluginhost "github.com/skeletg/skelet/apps/backend/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/internal/revalidate"
	"github.com/skeletg/skelet/apps/backend/internal/review"
	"github.com/skeletg/skelet/apps/backend/internal/scheduler"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/signalsboot"
	"github.com/skeletg/skelet/apps/backend/internal/sitecrawl"
	"github.com/skeletg/skelet/apps/backend/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/internal/webhook"
)

// Jobs owns the periodic-job engine - one engine runs every job; schedules are admin-configurable, persisted in scheduled_jobs, and claimed atomically so it's safe on every replica
// One ordered registration block rather than fx value groups - the jobs aren't owned by their domain packages, they're composition-root closures over several services each with env gates deciding which ones register at all, and a group:"jobs" fan-in would scatter the same closures across modules for no gain in isolation
// The outbox relay is deliberately NOT a job here - it's a 1s continuous drain, and a cron tick would 10x its event-delivery latency and serialize it
var Jobs = fx.Module("jobs", fx.Provide(newJobScheduler))

type jobDeps struct {
	fx.In

	Ctx     RootContext
	Wcfg    Config
	Queries *db.Queries

	Config       *config.Service
	Audit        *audit.Service
	Notification *notification.Service
	SiteCrawl    *sitecrawl.Service
	LinkGraph    *linkgraph.Service
	Webhook      *webhook.Service
	Catalog      *catalog.Service
	MediaLibrary *medialibrary.Service
	Subscription *subscription.Service
	Lens         *lens.Service
	Chat         *chat.Service
	AI           *ai.Service
	Review       *review.Service
	Campaign     *campaign.Service
	Flow         *flow.Service
	Inventory    *inventory.Service
	CacheQueue   *revalidate.Queue
	Cart         *cart.Service
	Channel      *channel.Service
	URLRewrite   *urlrewrite.Service
	CMS          *cms.Service
	Blog         *blog.Service
	Currency     *currency.Service
	DataJob      *datajob.Service
	Segment      *segment.Service
	Backup       *backup.Service
	Destination  *destination.Service
	Recorder     *analytics.Recorder
	Order        *order.Service
	Payment      *paymentpkg.Service

	SignalsEnabled SignalsEnabled
}

func newJobScheduler(d jobDeps) (*scheduler.Service, error) {
	bootCtx := d.Ctx.Context // never named ctx - every handler below takes its own, and one shadowing the other is how a job ends up running on the boot context
	jobScheduler := scheduler.New(d.Queries, d.Wcfg.Env != "production")
	// signalsJob registers a Signals-suite job only when SIGNALS_ENABLED is on - campaign/flow/analytics sweeps have nothing to do when the whole suite is off, while retention/prune jobs stay registered regardless (hygiene on existing data)
	signalsJob := func(name, spec, desc string, h scheduler.HandlerFunc) {
		if bool(d.SignalsEnabled) {
			jobScheduler.Register(name, spec, desc, h)
		}
	}
	// retentionJob: a Signals retention job (runs regardless of SIGNALS_ENABLED so old data still ages out) that cmd/signals owns when SIGNALS_JOBS_EXTERNAL
	retentionJob := func(name, spec, desc string, h scheduler.HandlerFunc) {
		if !d.Wcfg.SignalsJobsExternal {
			jobScheduler.Register(name, spec, desc, h)
		}
	}
	// signalsWorkerJob: a Signals job cmd/signals owns - register here only when Signals is on AND the worker isn't running it
	signalsWorkerJob := func(name, spec, desc string, h scheduler.HandlerFunc) {
		if bool(d.SignalsEnabled) && !d.Wcfg.SignalsJobsExternal {
			jobScheduler.Register(name, spec, desc, h)
		}
	}
	jobScheduler.SetPollInterval(15 * time.Second)
	schedCfg := scheduler.NewConfig(d.Config) // Global pause switch + failure-alert webhook (config-backed)
	jobScheduler.SetConfig(schedCfg)
	jobScheduler.Register("scheduler-history-prune", "@every 24h", "Delete scheduled-job run history past the retention window.",
		func(ctx context.Context) error {
			_, err := jobScheduler.PruneRuns(ctx, time.Now().AddDate(0, 0, -schedCfg.HistoryRetentionDays(ctx)))
			return err
		})
	// The other append-forever operational logs, same shape: config-driven window, <= 0 keeps forever
	jobScheduler.Register("audit-log-prune", "@every 24h", "Delete admin audit-log entries past the retention window.",
		func(ctx context.Context) error {
			days, err := d.Config.GetInt(ctx, audit.CfgRetentionDays, config.DefaultScope)
			if err != nil || days <= 0 {
				return err
			}
			_, err = d.Audit.Prune(ctx, time.Now().AddDate(0, 0, -int(days)))
			return err
		})
	jobScheduler.Register("notifications-prune", "@every 24h", "Delete read admin notifications past the retention window (never-read ones at 6x).",
		func(ctx context.Context) error {
			days, err := d.Config.GetInt(ctx, notification.CfgRetentionDays, config.DefaultScope)
			if err != nil || days <= 0 {
				return err
			}
			now := time.Now()
			_, err = d.Notification.Prune(ctx, now.AddDate(0, 0, -int(days)), now.AddDate(0, 0, -int(days)*6))
			return err
		})
	jobScheduler.Register("webhook-deliveries-prune", "@every 24h", "Delete webhook delivery-log entries past the retention window.",
		func(ctx context.Context) error {
			days, err := d.Config.GetInt(ctx, webhook.CfgDeliveryRetentionDays, config.DefaultScope)
			if err != nil || days <= 0 {
				return err
			}
			_, err = d.Webhook.PruneDeliveries(ctx, time.Now().AddDate(0, 0, -int(days)))
			return err
		})
	jobScheduler.Register("media-cleanup", "@every 2h", "Delete unconfirmed (orphaned) media uploads older than 24h.",
		func(ctx context.Context) error { d.Catalog.CleanupPendingMedia(ctx, 24*time.Hour); return nil })
	jobScheduler.Register("option-uploads-prune", "@every 24h", "Delete customer product-option file uploads older than 7 days that no cart or order line references.",
		func(ctx context.Context) error {
			_, err := d.MediaLibrary.CleanupProductOptionUploads(ctx, 7*24*time.Hour, 500)
			return err
		})
	// The reconciliation half of redirect/BNPL flows - a shopper who closes the tab mid-3DS leaves an
	// unpaid order no webhook will ever settle, so this asks the gateway directly and applies the answer
	// through the same amount-aware settle path the webhooks use
	jobScheduler.Register("payment-sync-sweep", "@every 15m", "Ask gateways where stuck unpaid gateway payments stand (abandoned redirect/3DS flows) and settle what they report.",
		func(ctx context.Context) error { return sweepUnsettledPayments(ctx, d.Order, d.Payment) })
	jobScheduler.Register("subscription-billing", "@every 1m", "Bill subscriptions whose next billing date has passed.",
		func(ctx context.Context) error { _, err := d.Subscription.ProcessDue(ctx, time.Now()); return err })
	jobScheduler.Register("subscription-collect", "@every 1m", "Charge due subscription invoices against saved payment methods (dunning ladder on failure).",
		func(ctx context.Context) error { _, err := d.Subscription.CollectDue(ctx, time.Now()); return err })
	jobScheduler.Register("blog-publish", "@every 1m", "Publish scheduled blog posts whose publish time has arrived.",
		func(ctx context.Context) error { _, err := d.Blog.PromoteDuePosts(ctx); return err })
	jobScheduler.Register("cms-publish", "@every 1m", "Publish scheduled CMS pages whose publish time has arrived.",
		func(ctx context.Context) error { _, err := d.CMS.PromoteDuePages(ctx); return err })
	jobScheduler.Register("cache-rebuild", "@every 10s", "Drain the page-rebuild queue, warming pages with bounded concurrency.",
		func(ctx context.Context) error { _, err := d.CacheQueue.ProcessDue(ctx, 25, 5); return err })
	jobScheduler.Register("cache-rebuild-prune", "@every 6h", "Delete finished (done/failed) page-rebuild tasks older than 7 days.",
		func(ctx context.Context) error { _, err := d.CacheQueue.Prune(ctx, 7*24*time.Hour); return err })
	jobScheduler.Register("data-transfer-sweep", "@every 1h", "Delete import/export files past their retention window, and prune long-finished job rows. The rows outlive their files so the history stays readable.",
		d.DataJob.Sweep)
	if d.AI.Enabled() { // skipped entirely under AI_ENABLED=false - the scheduler parks the orphaned job row and revives it on the next AI-enabled boot
		jobScheduler.Register("review-summary-refresh", "@every 10m", "Regenerate AI 'what customers say' summaries for products whose reviews changed (no-op when AI is off).",
			func(ctx context.Context) error { _, err := d.Review.RefreshDueSummaries(ctx, 10); return err })
	}
	jobScheduler.Register("reservation-prune", "@every 24h", "Delete settled (net-zero) inventory reservation chains past the retention window. Retention must exceed the return window - returns restock from these rows.",
		func(ctx context.Context) error {
			days, err := d.Config.GetInt(ctx, "sales/inventory/reservations_retention_days", config.DefaultScope)
			if err != nil || days <= 0 {
				return err
			}
			_, err = d.Inventory.PruneSettledReservations(ctx, time.Duration(days)*24*time.Hour)
			return err
		})
	jobScheduler.RegisterDisabled("seo-crawl-nightly", "0 3 * * *", "Crawl each storefront that opted in, then compare the result with the previous crawl and notify on regressions.",
		nightlyCrawl(seoCrawlDeps{Crawl: d.SiteCrawl, Graph: d.LinkGraph, Config: d.Config, Notification: d.Notification}))
	jobScheduler.Register("also-bought-rebuild", "@every 24h", "Recompute the customers-also-bought co-occurrence table from order history.",
		func(ctx context.Context) error { return d.Catalog.RebuildAlsoBought(ctx) })
	jobScheduler.Register("currency-rates", "@every 12h", "Refresh currency exchange rates from ECB (frankfurter.dev) against the platform base currency.",
		func(ctx context.Context) error {
			base, _ := d.Config.GetString(ctx, "general/store_information/currency_list", config.DefaultScope)
			if base == "" {
				base = "USD"
			}
			return d.Currency.RefreshRates(ctx, base)
		})
	signalsJob("campaign-send", "@every 1m", "Materialize + send due email campaigns to their audiences.",
		func(ctx context.Context) error { return d.Campaign.ProcessDue(ctx) })
	jobScheduler.Register("flow-engine", "@every 1m", "Advance marketing automation flow runs (waits, branches, sends).", // always registered - with Signals off the engine still advances TRANSACTIONAL flow runs (order receipts, auth links), marketing runs park inside advanceWithFlow
		func(ctx context.Context) error { return d.Flow.ProcessRuns(ctx) })
	jobScheduler.Register("cart-retention", "@every 24h", "Delete carts untouched past the retention window (abandoned guest carts + converted carts whose record lives on the order).",
		func(ctx context.Context) error {
			days, err := d.Config.GetInt(ctx, "sales/cart/retention_days", config.DefaultScope)
			if err != nil || days <= 0 {
				return err
			}
			n, err := d.Cart.PruneStale(ctx, time.Duration(days)*24*time.Hour)
			if n > 0 {
				log.Printf("[cart] pruned %d stale carts", n)
			}
			return err
		})
	jobScheduler.Register("urlrewrite-redirect-retention", "@every 24h", "Delete system-generated rename redirects (from URL auto-moves) past the retention window. Operator-authored redirects are kept.",
		func(ctx context.Context) error {
			days, err := d.Config.GetInt(ctx, "general/urls/redirect_retention_days", config.DefaultScope)
			if err != nil || days <= 0 {
				return err
			}
			n, err := d.URLRewrite.PruneStaleRedirects(ctx, time.Duration(days)*24*time.Hour)
			if n > 0 {
				log.Printf("[urlrewrite] pruned %d stale auto redirects", n)
			}
			return err
		})
	signalsJob("cart-abandonment", "@every 5m", "Enrol customers with idle non-empty carts into active abandoned-cart flows.",
		func(ctx context.Context) error {
			mins, _ := d.Config.GetInt(ctx, "pulse/flows/abandoned_cart_minutes", config.DefaultScope)
			maxAgeH, _ := d.Config.GetInt(ctx, "pulse/flows/abandoned_cart_max_age_hours", config.DefaultScope)
			if mins <= 0 {
				mins = 60
			}
			if maxAgeH <= 0 {
				maxAgeH = 72
			}
			now := time.Now()
			carts, err := d.Cart.ListAbandoned(ctx, now.Add(-time.Duration(mins)*time.Minute), now.Add(-time.Duration(maxAgeH)*time.Hour), 500)
			if err != nil {
				return err
			}
			done := make([]uuid.UUID, 0, len(carts))
			for _, c := range carts {
				d.Flow.OnCartAbandoned(ctx, c.ChannelID, c.CustomerID, c.Email, c.ID)
				done = append(done, c.ID)
			}
			return d.Cart.MarkAbandonedNotified(ctx, done)
		})
	signalsJob("browse-abandonment", "@every 15m", "Enrol customers who browsed products without carting or buying into browse-abandonment flows.",
		func(ctx context.Context) error {
			mins, _ := d.Config.GetInt(ctx, "pulse/flows/browse_abandoned_minutes", config.DefaultScope)
			maxAgeH, _ := d.Config.GetInt(ctx, "pulse/flows/browse_abandoned_max_age_hours", config.DefaultScope)
			if mins <= 0 {
				mins = 120
			}
			if maxAgeH <= 0 {
				maxAgeH = 48
			}
			now := time.Now()
			rows, err := d.Lens.ListBrowseAbandoners(ctx, now.Add(-time.Duration(mins)*time.Minute), now.Add(-time.Duration(maxAgeH)*time.Hour), 500)
			if err != nil {
				return err
			}
			for _, b := range rows {
				d.Flow.OnBrowseAbandoned(ctx, b.ChannelID, b.CustomerID, b.PageKey, b.Sku, b.ProductName, b.ProductImage)
				if err := d.Lens.MarkBrowseNotified(ctx, b.ChannelID, b.CustomerID, b.LastBrowseAt); err != nil {
					return err
				}
			}
			return nil
		})
	// DB backups: pg_dump streamed straight to the bucket named by the BACKUP_S3_* env (deliberately env-pinned, not admin config - a runtime-editable destination is a full-database exfiltration channel), never to local disk - `skelet db backup` uses the same env - direct DATABASE_URL since pg_dump must not go through PgBouncer txn pooling, and a full dump + upload can easily outlast the 5-minute default
	jobScheduler.RegisterWithTimeout("db-backup", "0 3 * * *", "Stream a pg_dump of the databases to the object storage named by the BACKUP_S3_* environment.", 30*time.Minute,
		func(ctx context.Context) error {
			if !d.Backup.Enabled() {
				return nil // not configured - a quiet no-op, not a failure
			}
			res, err := d.Backup.Run(ctx)
			if err != nil {
				return err
			}
			slog.Info("db-backup: uploaded", "key", res.Key, "bytes", res.Bytes, "signals_key", res.SignalsKey, "signals_bytes", res.SignalsBytes, "pruned", len(res.Pruned))
			return nil
		})
	// The unconfigured job no-ops as "success", so nothing downstream would ever flag a prod store running with no backups - say it loudly once at boot instead
	if d.Wcfg.Env == "production" {
		switch {
		case !d.Backup.Enabled():
			slog.Warn("db-backup: backups are NOT CONFIGURED - set the BACKUP_S3_* environment variables")
		case !d.Backup.Encrypted():
			slog.Warn("db-backup: dumps upload UNENCRYPTED - set BACKUP_ENCRYPTION_PASSPHRASE (and keep it safe: losing it loses the backups)")
		}
	}
	// Postgres has no TTL, so the auth-token fallback tables need sweeping - always registered - expiry is enforced on read anyway, this just reclaims space and no-ops when Valkey holds the tokens (tables stay empty)
	jobScheduler.Register("auth-token-prune", "@every 1h", "Delete expired rows from the Postgres auth-token fallback tables (no-op when Valkey is configured).",
		func(ctx context.Context) error {
			n, err := d.Queries.PruneExpiredAuthTokens(ctx)
			if err != nil {
				return fmt.Errorf("prune auth tokens: %w", err)
			}
			m, err := d.Queries.PruneExpiredRefreshTokens(ctx, time.Now().Add(-24*time.Hour)) // rotated rows outlive their expiry until the reuse window lapses, so a replay of a token that expired after rotation still trips the tripwire
			if err != nil {
				return fmt.Errorf("prune refresh tokens: %w", err)
			}
			if n+m > 0 {
				slog.Info("auth token prune", "auth_tokens", n, "refresh_tokens", m)
			}
			return nil
		})
	retentionJob("analytics-prune", "@every 24h", "Age out analytics event-spine rows and whole send-log months past their retention windows - deleted, or exported to the archive destination first under retention_action=archive.",
		signalsboot.SpinePruneJob(d.Config, d.Recorder))
	signalsWorkerJob("event-destinations-forward", "@every 1m", "Forward analytics-spine events to configured external destinations (webhooks, GA4, Meta CAPI) with durable per-destination cursors.",
		func(ctx context.Context) error {
			return d.Destination.ForwardAll(ctx)
		})
	signalsJob("synapse-segment-refresh", "@every 6h", "Re-materialize Synapse behavioral/trait segment memberships; auto-create attribute-affinity segments for opted-in attributes; enroll new members into segment_entered flows.",
		func(ctx context.Context) error {
			now := time.Now()
			if attrs, err := d.Catalog.ListAttributes(ctx); err == nil { // auto-create/refresh one segment per popular value of every attribute flagged synapse_track && auto_segment (e.g. "Tire size: 205/55R16")
				var auto []analytics.AffinityAttr
				for _, a := range attrs {
					if a.SynapseTrack && a.AutoSegment {
						auto = append(auto, analytics.AffinityAttr{Slug: a.Slug, Name: a.Name})
					}
				}
				if len(auto) > 0 {
					cids := []uuid.UUID{}
					if chs, err := d.Channel.List(ctx); err == nil {
						for _, ch := range chs {
							cids = append(cids, ch.ID)
						}
					}
					cfg := func(key string, def int) int {
						if v, err := d.Config.GetInt(ctx, key, config.DefaultScope); err == nil && v > 0 {
							return int(v)
						}
						return def
					}
					_, _, _ = graphadmin.SyncAffinitySegments(ctx, d.Recorder, d.Segment, cids, auto, // best-effort - a sync failure must not abort the membership refresh below
						cfg("pulse/synapse/affinity_segment_min_actors", 5),
						cfg("pulse/synapse/affinity_segment_max_values", 20),
						cfg("pulse/synapse/affinity_segment_window_days", 90), now)
				}
			}
			if segs, members, derr := d.Segment.RefreshAllDynamic(ctx); derr != nil {
				log.Printf("[segments] dynamic refresh: %d segments, %d members, errors: %v", segs, members, derr)
			}
			_, err := graphadmin.RefreshDefinedSegments(ctx, d.Recorder, d.Segment, now, func(ch, cust uuid.UUID, slug string) {
				d.Flow.OnSegmentEntered(ctx, ch, cust, slug)
			})
			return err
		})
	signalsWorkerJob("trait-compute", "@every 12h", "Recompute per-customer RFM / lifecycle / engagement traits (Synapse → shared across Signals).", // body shared with cmd/signals, which owns this job under SIGNALS_JOBS_EXTERNAL - pure DB compute with no flow-enrollment side effects, unlike the two jobs above
		signalsboot.TraitComputeJob(d.Recorder, d.Channel, d.Config))
	retentionJob("lens-prune", "@every 24h", "Age out Lens web-analytics events + heatmap grids + page snapshots past the retention window (replay and per-day rollups run their own windows).",
		signalsboot.LensPruneJob(d.Config, d.Lens))
	if d.AI.Enabled() {
		jobScheduler.Register("chat-prune", "@every 24h", "Delete chat conversations untouched past the retention window (0 days = keep forever).",
			func(ctx context.Context) error {
				days, err := d.Config.GetInt(ctx, "ai/llm/history_retention_days", config.DefaultScope)
				if err != nil || days <= 0 {
					return err
				}
				_, err = d.Chat.PruneBefore(ctx, time.Now().AddDate(0, 0, -int(days)))
				return err
			})
	}
	jobScheduler.Register("plugin-event-dedup-prune", "@every 24h", "Delete plugin event-dedup markers older than 30 days.", // 30 days is far past any outbox redelivery horizon, so a pruned marker can never un-dedupe a live replay
		func(ctx context.Context) error {
			_, err := d.Queries.PrunePluginEventDedup(ctx, time.Now().AddDate(0, 0, -30))
			return err
		})
	jobScheduler.Register("notification-scan", "@every 5m", "Scan standing ops conditions (failing scheduled jobs, plugin handler failures, …) and post them to the admin inbox.",
		func(ctx context.Context) error {
			if err := d.Notification.Scan(ctx); err != nil {
				return err
			}
			return notifyPluginAsyncFailures(ctx, d.Notification)
		})
	for _, pj := range pluginhost.ScheduledJobs() { // plugin jobs (sdk.RegisterScheduledJob) drain after core jobs, before Seed, so they get a row + first next_run like any code job
		jobScheduler.Register(pj.Name, pj.Schedule, pj.Description, scheduler.HandlerFunc(pj.Handler))
	}
	if err := jobScheduler.Seed(bootCtx); err != nil { // Seed runs in both roles - it only upserts (ON CONFLICT DO NOTHING) and keeps descriptions/defaults current for the admin jobs UI
		return nil, fmt.Errorf("seed scheduled jobs: %w", err)
	}
	return jobScheduler, nil
}

// Async plugin handler errors are logged and dropped by design - this is the operator-visible half: the scan drains the host's tally into the admin inbox, deduped per (plugin, event)
func notifyPluginAsyncFailures(ctx context.Context, n *notification.Service) error {
	var errs []error
	for _, f := range pluginhost.TakeAsyncFailures() {
		lastErr := f.LastError
		if len(lastErr) > 300 {
			lastErr = lastErr[:300] + "…"
		}
		errs = append(errs, n.Create(ctx, notification.Input{
			Category:           notification.CategorySystem,
			Severity:           notification.SeverityCritical,
			Title:              "Plugin " + f.PluginSlug + ": event handler failing",
			Body:               fmt.Sprintf("%d failure(s) on %s since the last scan. Latest: %s", f.Count, f.EventType, lastErr),
			RequiredPermission: notification.PermSystem,
			EntityType:         "plugin",
			EntityID:           f.PluginSlug,
			ActionURL:          "/plugins",
			DedupeKey:          "plugin_async_error:" + f.PluginSlug + ":" + f.EventType,
		}))
	}
	return errors.Join(errs...)
}
