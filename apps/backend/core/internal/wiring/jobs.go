package wiring

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/audit"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/chat"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/datajob"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/linkgraph"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/internal/notification"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	paymentpkg "github.com/skeletg/skelet/apps/backend/core/internal/payment"
	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/revalidate"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/scheduler"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/sitecrawl"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Jobs owns the periodic-job engine - one engine runs every job; schedules are admin-configurable, persisted in scheduled_jobs, and claimed atomically so it's safe on every replica One ordered registration block rather than fx value groups - the jobs aren't owned by their domain packages, they're composition-root closures over several services each with env gates deciding which ones register at all, and a group:"jobs" fan-in would scatter the same closures across modules for no gain in isolation The outbox relay is deliberately NOT a job here - it's a 1s continuous drain, and a cron tick would 10x its event-delivery latency and serialize it
var Jobs = fx.Module("jobs", fx.Provide(newJobScheduler))

type jobDeps struct {
	fx.In

	Ctx     RootContext
	Wcfg    Config
	Queries *db.Queries

	Config        *config.Service
	Audit         *audit.Service
	Notification  *notification.Service
	SiteCrawl     *sitecrawl.Service
	LinkGraph     *linkgraph.Service
	Webhook       *webhook.Service
	Catalog       *catalog.Service
	MediaLibrary  *medialibrary.Service
	Subscription  *subscription.Service
	Chat          *chat.Service
	AI            *ai.Service
	Review        *review.Service
	Flows         *FlowEnroller
	Inventory     *inventory.Service
	CacheQueue    *revalidate.Queue
	Cart          *cart.Service
	Channel       *channel.Service
	URLRewrite    *urlrewrite.Service
	CMS           *cms.Service
	Blog          *blog.Service
	Currency      *currency.Service
	DataJob       *datajob.Service
	SignalsClient *signalsclient.Client
	Order         *order.Service
	Payment       *paymentpkg.Service

	SignalsEnabled SignalsEnabled
}

func newJobScheduler(d jobDeps) (*scheduler.Service, error) {
	bootCtx := d.Ctx.Context // never named ctx - every handler below takes its own, and one shadowing the other is how a job ends up running on the boot context
	jobScheduler := scheduler.New(d.Queries, d.Wcfg.Env != "production")
	// signalsJob registers a Signals-suite job only when SIGNALS_ENABLED is on - campaign/flow/analytics sweeps have nothing to do when the whole suite is off, while retention/prune jobs stay registered regardless (hygiene on existing data)
	// How long an engine tick may keep draining before it answers - inside the minute, so the next tick is never late
	const engineTickBudget = 45 * time.Second
	signalsJob := func(name, spec, desc string, h scheduler.HandlerFunc) {
		if bool(d.SignalsEnabled) {
			jobScheduler.Register(name, spec, desc, h)
		}
	}
	// The suite performs these on request, so they exist only where there is a suite to ask - a nil client would fail every tick and alert on each one
	retentionJob := func(name, spec, desc string, h scheduler.HandlerFunc) {
		if d.SignalsClient != nil {
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
	// The reconciliation half of redirect/BNPL flows - a shopper who closes the tab mid-3DS leaves an unpaid order no webhook will ever settle, so this asks the gateway directly and applies the answer through the same amount-aware settle path the webhooks use
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
	// Both engines run in the suite - core keeps the cadence, the run history and the operator's run-now button
	// Each tick drains for most of its minute, and the job lease holds the next tick behind it
	signalsJob("campaign-send", "@every 1m", "Materialize + send due email campaigns to their audiences.",
		func(ctx context.Context) error {
			n, err := d.SignalsClient.ProcessDueCampaigns(ctx, engineTickBudget)
			if n > 0 {
				slog.Info("campaign-send: recipients handled", "count", n)
			}
			return err
		})
	signalsJob("flow-engine", "@every 1m", "Advance marketing automation flow runs (waits, branches, sends).",
		func(ctx context.Context) error {
			n, err := d.SignalsClient.AdvanceFlowRuns(ctx, engineTickBudget)
			if n > 0 {
				slog.Info("flow-engine: runs advanced", "count", n)
			}
			return err
		})
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
			mins, _ := d.Config.GetInt(ctx, "outreach/triggers/abandoned_cart_minutes", config.DefaultScope)
			maxAgeH, _ := d.Config.GetInt(ctx, "outreach/triggers/abandoned_cart_max_age_hours", config.DefaultScope)
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
			// A cart is stamped told only once the suite answered for it - the rest wait for the next tick
			done := make([]uuid.UUID, 0, len(carts))
			var unreached error
			for _, c := range carts {
				if err := d.Flows.OnCartAbandoned(ctx, c.ChannelID, c.CustomerID, c.Email, c.ID, c.LastActivity); err != nil {
					unreached = err
					break
				}
				done = append(done, c.ID)
			}
			if err := d.Cart.MarkAbandonedNotified(ctx, done); err != nil {
				return err
			}
			return unreached
		})
	signalsJob("browse-abandonment", "@every 15m", "Enrol customers who browsed products without carting or buying into browse-abandonment flows.",
		func(ctx context.Context) error {
			mins, _ := d.Config.GetInt(ctx, "outreach/triggers/browse_abandoned_minutes", config.DefaultScope)
			maxAgeH, _ := d.Config.GetInt(ctx, "outreach/triggers/browse_abandoned_max_age_hours", config.DefaultScope)
			if mins <= 0 {
				mins = 120
			}
			if maxAgeH <= 0 {
				maxAgeH = 48
			}
			now := time.Now()
			// Lens says who browsed and went quiet; whether they then carted or bought is commerce's to answer,
			// so ask for ten times the enrolment and let the exclusion eat into the buffer
			const enrol = 500
			cands, err := d.SignalsClient.BrowseAbandoners(ctx, now.Add(-time.Duration(mins)*time.Minute), now.Add(-time.Duration(maxAgeH)*time.Hour), enrol*10)
			if err != nil {
				return err
			}
			if len(cands) == 0 {
				return nil
			}
			ids := make([]uuid.UUID, len(cands))
			ats := make([]time.Time, len(cands))
			for i, c := range cands {
				ids[i], ats[i] = c.CustomerID, c.LastBrowseAt
			}
			excludedRows, err := d.Queries.BrowseAbandonersExcluded(ctx, db.BrowseAbandonersExcludedParams{CustomerIds: ids, BrowseAts: ats})
			if err != nil {
				return err
			}
			excluded := make(map[uuid.UUID]bool, len(excludedRows))
			for _, id := range excludedRows {
				excluded[id] = true
			}
			sent := 0
			for _, b := range cands {
				if excluded[b.CustomerID] {
					// Answered for, so mark it told or the same rows fill every tick until the window ages out
					if err := d.SignalsClient.MarkBrowseNotified(ctx, b.ChannelID, b.CustomerID, b.LastBrowseAt); err != nil {
						return err
					}
					continue
				}
				// Marked told only on an answer, for the same reason the carts are
				if err := d.Flows.OnBrowseAbandoned(ctx, b.ChannelID, b.CustomerID, b.PageKey, b.Sku, b.ProductName, b.ProductImage, b.LastBrowseAt); err != nil {
					return err
				}
				if err := d.SignalsClient.MarkBrowseNotified(ctx, b.ChannelID, b.CustomerID, b.LastBrowseAt); err != nil {
					return err
				}
				if sent++; sent >= enrol {
					break
				}
			}
			return nil
		})
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
	retentionJob("analytics-prune", "@every 24h", "Age out analytics event-spine rows past the retention window - deleted, or exported to the archive destination first under retention_action=archive.",
		d.SignalsClient.PruneSpine)
	// Core, not a Signals job: the send log is a commerce table that every transactional mail writes, so it has to be
	// maintained on a server with Signals off and on one that hands its Signals jobs to cmd/signals
	retentionJob("send-log-retention", "@every 24h", "Pre-create the email send log's upcoming month partitions and drop whole months past the retention window - deleted, or exported to the archive destination first under retention_action=archive.",
		d.SignalsClient.PruneSendLog)
	signalsJob("event-destinations-forward", "@every 1m", "Forward analytics-spine events to configured external destinations (webhooks, GA4, Meta CAPI) with durable per-destination cursors.",
		d.SignalsClient.ForwardDestinations)
	// Order-count, spend, recency, consent and group rules are plain commerce facts, but the suite is what materializes them into memberships
	retentionJob("segment-refresh", "@every 6h", "Re-materialize dynamic customer-segment memberships from their rules (segments needing Synapse traits are skipped while Signals is off).",
		func(ctx context.Context) error {
			segs, members, reported, err := d.SignalsClient.RefreshDynamicSegments(ctx)
			if err != nil {
				return err
			}
			if reported != "" {
				log.Printf("[segments] dynamic refresh: %d segments, %d members, errors: %v", segs, members, reported)
			}
			return nil
		})
	signalsJob("synapse-segment-refresh", "@every 6h", "Re-materialize Synapse behavioral/trait segment memberships; auto-create attribute-affinity segments for opted-in attributes; new members start their segment_entered flows in the suite.",
		func(ctx context.Context) error {
			// Which attributes to auto-segment on is a catalog flag, so core names them and the suite does the work
			var auto []signalsclient.AffinityAttribute
			if attrs, err := d.Catalog.ListAttributes(ctx); err == nil {
				for _, a := range attrs {
					if a.SynapseTrack && a.AutoSegment {
						auto = append(auto, signalsclient.AffinityAttribute{Slug: a.Slug, Name: a.Name})
					}
				}
			}
			// Whoever newly joined an audience starts their segment_entered runs in the suite
			return d.SignalsClient.RefreshBehavioralSegments(ctx, auto)
		})
	// Core owns channels, so core pushes them - the suite's beacon surface resolves every request against its copy
	// Often, because a channel switched on or retired only takes effect on the next push
	signalsJob("channel-roster-push", "@every 5m", "Push core's active channels to Signals, which its beacon surface resolves every request against.",
		func(ctx context.Context) error { return PushChannelRoster(ctx, d.Channel, d.Config, d.SignalsClient) })
	// Core owns customers, so core pushes them - the suite sweeps what the stream did not carry, which is the cascade a foreign key cannot give it across two databases
	signalsJob("customer-roster-push", "@every 6h", "Stream core's customer list to Signals, so its trait and membership cleanup can tell a deleted customer from an absent one.",
		func(ctx context.Context) error {
			const page = 5000
			var off int32
			next := func() (signalsclient.RosterPage, bool, error) {
				rows, err := d.Queries.CustomerRosterForSync(ctx, db.CustomerRosterForSyncParams{Lim: page, Off: off})
				if err != nil || len(rows) == 0 {
					return signalsclient.RosterPage{}, false, err
				}
				off += page
				out := signalsclient.RosterPage{Customers: make([]signalsclient.RosterCustomer, len(rows))}
				for i, c := range rows {
					out.Customers[i] = signalsclient.RosterCustomer{
						CustomerID: c.CustomerID, ChannelID: c.ChannelID, Email: c.Email,
						FirstName: c.FirstName, Phone: c.Phone, IsActive: c.IsActive,
						GroupID:          uuid.NullUUID{UUID: c.CustomerGroupID, Valid: c.CustomerGroupID != uuid.Nil},
						GroupSlug:        c.CustomerGroupSlug,
						MarketingOptedIn: c.MarketingOptedIn, SMSOptedIn: c.SmsOptedIn,
					}
				}
				return out, true, nil
			}
			synced, pruned, err := d.SignalsClient.PushCustomerRoster(ctx, next)
			if err != nil {
				return err
			}
			if pruned > 0 {
				log.Printf("[roster] pushed %d customers, %d gone", synced, pruned)
			}
			return nil
		})
	signalsJob("trait-compute", "@every 12h", "Recompute per-customer RFM / lifecycle / engagement traits (Synapse → shared across Signals).",
		d.SignalsClient.ComputeTraits)
	retentionJob("lens-prune", "@every 24h", "Age out Lens web-analytics events + heatmap grids + page snapshots past the retention window (replay and per-day rollups run their own windows).",
		d.SignalsClient.PruneLens)
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

// PushChannelRoster hands the suite every channel core actively runs, leaving the inactive ones out
// The identity fields come from the config registry, so a message sent from the suite still looks like it came from the shop
func PushChannelRoster(ctx context.Context, chans *channel.Service, cfg *config.Service, client *signalsclient.Client) error {
	all, err := chans.List(ctx)
	if err != nil {
		return err
	}
	active := make([]signalsclient.ChannelEntry, 0, len(all))
	for _, ch := range all {
		if !ch.IsActive {
			continue
		}
		id := ch.ID
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &id}
		entry := signalsclient.ChannelEntry{ID: ch.ID, IsDefault: ch.IsDefault, Name: ch.Name, PublicURL: ch.PublicURL}
		if cfg != nil {
			// general/branding/store_name is the customer-facing name a mail prints; channels.name is the
			// operator's internal label ("US Wholesale B2B"), which is what the setting exists to keep out of one
			if name, _ := cfg.GetString(ctx, "general/branding/store_name", scope); strings.TrimSpace(name) != "" {
				entry.Name = strings.TrimSpace(name)
			}
			entry.FromAddress, _ = cfg.GetString(ctx, "general/contact/email_from_address", scope)
			entry.FromName, _ = cfg.GetString(ctx, "general/contact/email_from_name", scope)
			entry.LogoURL, _ = cfg.GetString(ctx, "general/branding/logo_url", scope)
		}
		active = append(active, entry)
	}
	if _, _, err := client.PushChannels(ctx, active); err != nil {
		return err
	}
	return nil
}
