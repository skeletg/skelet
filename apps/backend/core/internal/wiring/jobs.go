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

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/audit"
	"github.com/skeletg/skelet/apps/backend/core/internal/authstore"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
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
	"github.com/skeletg/skelet/apps/backend/core/internal/recommend"
	"github.com/skeletg/skelet/apps/backend/core/internal/revalidate"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/scheduler"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient/enroll"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient/roster"
	"github.com/skeletg/skelet/apps/backend/core/internal/signupform"
	"github.com/skeletg/skelet/apps/backend/core/internal/sitecrawl"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
)

// Jobs owns the periodic-job engine - one engine runs every job; schedules are admin-configurable, persisted in scheduled_jobs, and claimed atomically so it's safe on every replica One ordered registration block rather than fx value groups - the jobs aren't owned by their domain packages, they're composition-root closures over several services each with env gates deciding which ones register at all, and a group:"jobs" fan-in would scatter the same closures across modules for no gain in isolation The outbox relay is deliberately NOT a job here - it's a 1s continuous drain, and a cron tick would 10x its event-delivery latency and serialize it
var Jobs = fx.Module("jobs", fx.Provide(newJobScheduler))

type jobDeps struct {
	fx.In

	Lc      fx.Lifecycle
	Ctx     RootContext
	Wcfg    Config
	Queries *db.Queries

	Config        *config.Service
	Cipher        *secrets.Cipher
	AllowPrivate  AllowPrivateTargets
	Audit         *audit.Service
	Notification  *notification.Service
	SiteCrawl     *sitecrawl.Service
	LinkGraph     *linkgraph.Service
	Webhook       *webhook.Service
	Catalog       *catalog.Service
	Recommend     *recommend.Service
	MediaLibrary  *medialibrary.Service
	Subscription  *subscription.Service
	Chat          *ai.History
	AI            *ai.Service
	Review        *review.Service
	Flows         *enroll.Enroller
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
	Form          *signupform.Service

	SignalsEnabled SignalsEnabled
}

func newJobScheduler(d jobDeps) (*scheduler.Service, error) {
	bootCtx := d.Ctx.Context // never named ctx - every handler below takes its own, and one shadowing the other is how a job ends up running on the boot context
	// The webhooks package's own rule - any non-production env used to count, so a staging scheduler could reach private targets
	jobScheduler := scheduler.New(d.Queries, bool(d.AllowPrivate))
	jobScheduler.SetCipher(d.Cipher)
	// signalsJob registers a Signals-suite job only when SIGNALS_ENABLED is on - campaign/flow/analytics sweeps have nothing to do when the whole suite is off, while retention/prune jobs stay registered regardless (hygiene on existing data)
	// How long an engine tick may keep draining before it answers - inside the minute, so the next tick is never late
	const engineTickBudget = 45 * time.Second
	signalsJob := func(name, spec, desc string, h scheduler.HandlerFunc) {
		if bool(d.SignalsEnabled) {
			jobScheduler.RegisterWithTimeout(name, spec, desc, signalsJobTimeout(name), h)
		}
	}
	// The suite performs these on request, so they exist only where there is a suite to ask - a nil client would fail every tick and alert on each one
	retentionJob := func(name, spec, desc string, h scheduler.HandlerFunc) {
		if d.SignalsClient != nil {
			jobScheduler.RegisterWithTimeout(name, spec, desc, signalsJobTimeout(name), h)
		}
	}
	jobScheduler.SetPollInterval(15 * time.Second)
	if d.SignalsClient != nil && bool(d.SignalsEnabled) {
		d.Lc.Append(fx.Hook{OnStart: func(context.Context) error {
			go roster.ResyncIfBehind(bootCtx, d.SignalsClient, jobScheduler)
			return nil
		}})
	}
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
		func(ctx context.Context) error { return SweepUnsettledPayments(ctx, d.Order, d.Payment) })
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
	jobScheduler.Register("signup-confirmations-prune", "@every 24h", "Delete signup-form confirmation links that expired unconfirmed.",
		func(ctx context.Context) error { _, err := d.Form.PruneConfirmations(ctx); return err })
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
		func(ctx context.Context) error { return d.Recommend.RebuildAlsoBought(ctx) })
	jobScheduler.Register("currency-rates", "@every 12h", "Refresh currency exchange rates from ECB (frankfurter.dev) against the platform base currency.",
		func(ctx context.Context) error { return refreshCurrencyRates(ctx, d.Config, d.Currency) })
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
	jobScheduler.Register("cart-offer-expiry", "@every 5m", "Announce cart offers that reached their expiry unfinished (cart.offer.expired). The link stops working at the expiry itself; this only tells subscribers.",
		func(ctx context.Context) error {
			n, err := d.Cart.AnnounceExpiredOffers(ctx, 500)
			if n > 0 {
				log.Printf("[cart] %d offers expired", n)
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
			channels, err := activeChannelIDs(ctx, d.Channel)
			if err != nil {
				return err
			}
			return runCartAbandonment(ctx, channels, d.Config, d.Cart, d.Flows, time.Now())
		})
	signalsJob("browse-abandonment", "@every 15m", "Enrol customers who browsed products without carting or buying into browse-abandonment flows.",
		func(ctx context.Context) error {
			channels, err := activeChannelIDs(ctx, d.Channel)
			if err != nil {
				return err
			}
			return runBrowseAbandonment(ctx, channels, d.Config, d.SignalsClient, d.Queries, d.Flows, time.Now())
		})
	// Always registered: the Postgres token tables stay empty when Valkey holds the tokens, but admin sessions live in Postgres either way
	jobScheduler.Register("auth-token-prune", "@every 1h", "Delete expired auth tokens, lapsed refresh successors and admin sessions whose refresh token can no longer be used.",
		func(ctx context.Context) error {
			c, err := authstore.Prune(ctx, d.Queries, time.Now())
			if err != nil {
				return err
			}
			if c.Total() > 0 {
				slog.Info("auth token prune", "auth_tokens", c.AuthTokens, "refresh_tokens", c.RefreshTokens, "successors", c.Successors, "admin_sessions", c.Sessions)
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
	signalsJob("channel-roster-push", "@every 5m", "Push core's channels and whether each is active to Signals, which its beacon surface resolves every request against.",
		func(ctx context.Context) error { return roster.PushChannels(ctx, d.Channel, d.Config, d.SignalsClient) })
	// Core owns customers, so core pushes them - the suite sweeps what the stream did not carry, which is the cascade a foreign key cannot give it across two databases
	signalsJob("customer-roster-push", "@every 6h", "Stream core's customer list to Signals, so its trait and membership cleanup can tell a deleted customer from an absent one.",
		func(ctx context.Context) error {
			const page = 5000
			var after uuid.UUID
			next := func() (signalsclient.RosterPage, bool, error) {
				rows, err := d.Queries.CustomerRosterForSync(ctx, db.CustomerRosterForSyncParams{After: after, Lim: page})
				if err != nil || len(rows) == 0 {
					return signalsclient.RosterPage{}, false, err
				}
				after = rows[len(rows)-1].CustomerID
				out := signalsclient.RosterPage{Customers: make([]signalsclient.RosterCustomer, len(rows))}
				for i, c := range rows {
					out.Customers[i] = signalsclient.RosterCustomer{
						CustomerID: c.CustomerID, ChannelID: c.ChannelID, Email: c.Email,
						FirstName: c.FirstName, Phone: c.Phone, IsActive: c.IsActive,
						GroupID:          uuid.NullUUID{UUID: c.CustomerGroupID, Valid: c.CustomerGroupID != uuid.Nil},
						GroupSlug:        c.CustomerGroupSlug,
						Consents:         signalsclient.ConsentsFromJSON(c.Consents),
						MarketingOptedIn: c.MarketingOptedIn, SMSOptedIn: c.SmsOptedIn, ReadAt: c.ReadAt,
						OrderCount: c.OrderCount, MonetaryCents: c.MonetaryCents,
						FirstOrderAt: signalsclient.TimeFromAny(c.FirstOrderAt), LastOrderAt: signalsclient.TimeFromAny(c.LastOrderAt),
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
				days, err := d.Config.GetInt(ctx, ai.HistoryRetentionPath, config.DefaultScope)
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

func signalsJobTimeout(name string) time.Duration {
	switch name {
	case "trait-compute", "segment-refresh", "synapse-segment-refresh":
		return 30 * time.Minute
	}
	return 0
}
