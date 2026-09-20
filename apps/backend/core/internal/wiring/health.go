package wiring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/health"
	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/scheduler"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/backup"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

// Checks register HERE rather than in their own packages because a probe needs a
// live handle, and this is the only layer holding them all
var Health = fx.Module("health", fx.Provide(newHealthRegistry))

type healthDeps struct {
	fx.In

	Infra         Infra
	SearchTier    SearchTier
	MediaStorage  *objstore.Storage
	Scheduler     *scheduler.Service
	Webhook       *webhook.Service
	Backup        *backup.Service
	SignalsClient *signalsclient.Client
	SignalsOn     SignalsEnabled
	Config        *config.Service
}

func newHealthRegistry(d healthDeps) *health.Registry {
	r := health.NewRegistry()
	registerDatastoreChecks(r, d)
	registerSearchChecks(r, d)
	registerBackgroundChecks(r, d)
	registerDeliveryChecks(r, d)
	registerSignalsChecks(r, d)
	registerPluginChecks(r)
	registerBackupChecks(r, d)
	return r
}

func registerDatastoreChecks(r *health.Registry, d healthDeps) {
	r.RegisterGroup(health.Group{Key: "datastores", Label: "Datastores", Order: 1})

	r.Register(health.Check{Group: "datastores", Key: "db", Label: "Commerce database", Order: 1,
		Run: func(ctx context.Context) health.Result { return pingPool(ctx, d.Infra.DB, "") }})

	r.Register(health.Check{Group: "datastores", Key: "cache", Label: "Valkey (auth/primary)", Order: 3,
		Run: func(ctx context.Context) health.Result {
			if d.Infra.Valkey == nil {
				return health.Off("not configured", "Auth tokens fall back to Postgres and rate limits to per-process state, so they stop being shared across pods")
			}
			if err := d.Infra.Valkey.Ping(ctx).Err(); err != nil {
				return health.Bad("unreachable", err.Error())
			}
			return health.OK("reachable")
		}})

	// Only distinct when VALKEY_CACHE_URL points somewhere else - otherwise the cache shares the primary's client and the check above already covers it
	if d.Infra.Cache.Client != nil && d.Infra.Cache.Client != d.Infra.Valkey {
		r.Register(health.Check{Group: "datastores", Key: "cache_evictable", Label: "Valkey (cache)", Order: 4,
			Run: func(ctx context.Context) health.Result {
				if err := d.Infra.Cache.Client.Ping(ctx).Err(); err != nil {
					return health.Bad("unreachable", err.Error()) // fail-open at runtime - caches and rate limits degrade rather than erroring so this is informational not a readiness gate
				}
				return health.OK("reachable")
			}})
	}

	r.Register(health.Check{Group: "datastores", Key: "storage", Label: "Object storage", Order: 5,
		Run: func(ctx context.Context) health.Result {
			if d.MediaStorage == nil {
				return health.Bad("not configured", "Media has no filesystem driver - every image read and upload fails without a bucket")
			}
			if _, _, _, err := d.MediaStorage.Head(ctx, ".skelet-health-probe"); err != nil { // a reachable bucket answers "absent" without error, so this proves connection + credentials without writing
				return health.Bad("unreachable", fmt.Sprintf("bucket %q: %v", d.MediaStorage.Bucket(), err))
			}
			return health.OK(fmt.Sprintf("bucket %q reachable", d.MediaStorage.Bucket()))
		}})
}

// Saturation matters more than reachability here - a pool at its ceiling still
// pings fine while every request queues behind it
func pingPool(ctx context.Context, pool *sql.DB, label string) health.Result {
	if pool == nil {
		return health.Bad("not configured", "No connection pool was wired"+label)
	}
	if err := pool.PingContext(ctx); err != nil {
		return health.Bad("unreachable", err.Error())
	}
	s := pool.Stats()
	detail := fmt.Sprintf("reachable - %d/%d connections in use", s.InUse, s.MaxOpenConnections)
	if s.MaxOpenConnections > 0 && s.InUse >= s.MaxOpenConnections {
		return health.Warn(detail, "The pool is exhausted - requests are queueing for a connection. Look for a slow query or a leaked transaction before raising the limit")
	}
	if s.WaitCount > 0 {
		return health.OK(detail + fmt.Sprintf(" (%d waited since boot)", s.WaitCount))
	}
	return health.OK(detail)
}

func registerSearchChecks(r *health.Registry, d healthDeps) {
	r.RegisterGroup(health.Group{Key: "search", Label: "Search", Order: 2})

	// Which engine ANSWERS, not which is configured - browse is the only listing path, so an outage here reads as an empty catalogue rather than an error
	r.Register(health.Check{Group: "search", Key: "tier", Label: "Serving tier", Order: 1,
		Run: func(ctx context.Context) health.Result {
			if d.SearchTier.Engine == nil {
				return health.Bad("none", "No search engine was wired - product listings cannot be served")
			}
			name := d.SearchTier.Engine.Name()
			if !strings.Contains(name, "opensearch") {
				return health.OK(name + " (no cluster configured)")
			}
			return health.OK(name)
		}})

	r.Register(health.Check{Group: "search", Key: "opensearch", Label: "OpenSearch cluster", Order: 2,
		Run: func(ctx context.Context) health.Result {
			if d.SearchTier.Client == nil {
				return health.Off("not configured",
					"Browse and search run on the Postgres tier: ts_rank relevance, no stemming, no merchandising boosts, and facets on selectable attributes only")
			}
			if err := d.SearchTier.Client.Ping(ctx); err != nil {
				return health.Bad("unreachable", err.Error()+" - listings are being served from the Postgres tier, so results and facets are degraded but the store is still shoppable")
			}
			return health.OK("reachable")
		}})
}

func registerBackgroundChecks(r *health.Registry, d healthDeps) {
	r.RegisterGroup(health.Group{Key: "background", Label: "Background work", Order: 3})

	// A wedged claim loop looks like nothing from the outside - jobs just stop, visible only by reading the job list
	r.Register(health.Check{Group: "background", Key: "scheduler", Label: "Scheduled jobs", Order: 1,
		Run: func(ctx context.Context) health.Result {
			jobs, err := d.Scheduler.List(ctx)
			if err != nil {
				return health.Bad("unreadable", err.Error())
			}
			enabled, overdue, failing := 0, 0, 0
			var worstName string
			var worstDue time.Duration
			for _, j := range jobs {
				if !j.Enabled {
					continue
				}
				enabled++
				if j.ConsecutiveFailures > 0 {
					failing++
				}
				if j.NextRunAt != nil {
					if late := time.Since(*j.NextRunAt); late > time.Minute { // a minute of slack absorbs the poll interval without crying wolf
						overdue++
						if late > worstDue {
							worstDue, worstName = late, j.Name
						}
					}
				}
			}
			if enabled == 0 {
				return health.Off("none enabled", "")
			}
			detail := fmt.Sprintf("%d enabled", enabled)
			switch {
			case overdue > 0:
				return health.Bad(fmt.Sprintf("%s, %d overdue", detail, overdue),
					fmt.Sprintf("%q is %s past due. The claim loop may not be running - check that a process with the jobs role is alive", worstName, roundDur(worstDue)))
			case failing > 0:
				return health.Warn(fmt.Sprintf("%s, %d failing", detail, failing),
					"A job has consecutive failures. Open Scheduled jobs for the last error on each")
			}
			return health.OK(detail + ", none overdue")
		}})
}

func registerDeliveryChecks(r *health.Registry, d healthDeps) {
	r.RegisterGroup(health.Group{Key: "delivery", Label: "Delivery", Order: 4})

	r.Register(health.Check{Group: "delivery", Key: "webhooks", Label: "Webhooks", Order: 1,
		Run: func(ctx context.Context) health.Result {
			hooks, err := d.Webhook.List(ctx)
			if err != nil {
				return health.Bad("unreadable", err.Error())
			}
			active, failing := 0, 0
			var worst string
			for _, h := range hooks {
				if !h.IsActive {
					continue
				}
				active++
				stats, serr := d.Webhook.DeliveryStats(ctx, h.ID)
				if serr != nil || stats.LastFailureAt == nil {
					continue
				}
				if stats.LastSuccessAt == nil || stats.LastFailureAt.After(*stats.LastSuccessAt) { // only the LATEST attempt counts - a failure that has since recovered isn't worth attention
					failing++
					if worst == "" {
						worst = h.TargetURL
					}
				}
			}
			if active == 0 {
				return health.Off("none active", "")
			}
			if failing > 0 {
				return health.Warn(fmt.Sprintf("%d active, %d failing", active, failing),
					fmt.Sprintf("The last attempt to %s failed. Deliveries retry into the dead-letter queue, so nothing is lost yet", worst))
			}
			return health.OK(fmt.Sprintf("%d active, last attempt succeeded on each", active))
		}})

	// The destination rows carry their own last error, so this is a read rather than a probe
	r.Register(health.Check{Group: "delivery", Key: "destinations", Label: "Event forwarding", Order: 2,
		Run: func(ctx context.Context) health.Result {
			if !bool(d.SignalsOn) {
				return health.Off("disabled", "SIGNALS_ENABLED is false, so nothing is forwarded")
			}
			h, err := d.SignalsClient.Health(ctx)
			if err != nil {
				return health.Bad("unreadable", err.Error())
			}
			if h.DestinationsError != "" {
				return health.Bad("unreadable", h.DestinationsError)
			}
			dests := h.Destinations
			if len(dests) == 0 {
				return health.Off("no destinations", "")
			}
			var broken []string
			var quietest *signalsclient.DestinationHealth
			for i, dest := range dests {
				if dest.LastError != "" {
					broken = append(broken, dest.Name)
				}
				if quietest == nil || olderForward(dest.LastForwardedAt, quietest.LastForwardedAt) {
					quietest = &dests[i]
				}
			}
			if len(broken) > 0 {
				return health.Bad(fmt.Sprintf("%d active, %d erroring", len(dests), len(broken)),
					strings.Join(broken, ", ")+" reported an error on the last send")
			}
			if quietest != nil && hoursSince(quietest.LastForwardedAt) > 1 {
				return health.Warn(fmt.Sprintf("%d active, quietest sent %s", len(dests), agoText(quietest.LastForwardedAt)),
					"No delivery in over an hour. Fine if the store is quiet, otherwise check the receiver")
			}
			return health.OK(fmt.Sprintf("%d active", len(dests)))
		}})
}

// Each probe asks the suite for the same snapshot - the registry memoises it, so the repeats cost one in-cluster round trip each
// A suite switched off is a deployment shape, so the probes below say so rather than reporting a fault
var errSignalsOff = errors.New("signals is switched off")

func registerSignalsChecks(r *health.Registry, d healthDeps) {
	r.RegisterGroup(health.Group{Key: "signals", Label: "Signals pipeline", Order: 5})

	// A suite switched off is a deployment shape, not a fault, so every probe here reports Off rather than failing
	suite := func(ctx context.Context) (signalsclient.SuiteHealth, error) {
		if !bool(d.SignalsOn) {
			return signalsclient.SuiteHealth{}, errSignalsOff
		}
		return d.SignalsClient.Health(ctx)
	}

	r.Register(health.Check{Group: "signals", Key: "enabled", Label: "Signals", Order: 1,
		Run: func(ctx context.Context) health.Result {
			if _, err := suite(ctx); err != nil {
				if errors.Is(err, errSignalsOff) {
					return health.Off("disabled", "SIGNALS_ENABLED is false, so nothing here runs")
				}
				return health.Bad("unreachable", err.Error())
			}
			return health.OK("reachable")
		}})

	r.Register(health.Check{Group: "signals", Key: "spine", Label: "Event spine", Order: 2,
		Run: func(ctx context.Context) health.Result {
			h, err := suite(ctx)
			if err != nil {
				return health.Bad("unreachable", err.Error())
			}
			if h.SpineError != "" {
				return health.Bad(h.SpineEngine+" unreachable", h.SpineError)
			}
			return health.OK(h.SpineEngine + " reachable")
		}})

	r.Register(health.Check{Group: "signals", Key: "ingest", Label: "Event ingest", Order: 3,
		Run: func(ctx context.Context) health.Result {
			h, err := suite(ctx)
			if err != nil {
				return health.Bad("unreachable", err.Error())
			}
			if h.SpineError != "" {
				return health.Bad("unknown", h.SpineError)
			}
			if h.SpineNewest == nil {
				return health.Warn("no events recorded", "Nothing has ever arrived. If the storefront is live, check that /t reaches the backend")
			}
			if hoursSince(h.SpineNewest) > 24 {
				return health.Warn("last event "+agoText(h.SpineNewest), "Nothing has arrived in over a day. If the storefront is live, check that /t reaches the backend")
			}
			return health.OK("last event " + agoText(h.SpineNewest))
		}})

	r.Register(health.Check{Group: "signals", Key: "retention", Label: "Retention", Order: 4,
		Run: func(ctx context.Context) health.Result {
			days, err := d.Config.GetInt(ctx, "signals/analytics/spine_retention_days", config.DefaultScope)
			if err != nil || days <= 0 {
				return health.Off("not configured", "")
			}
			h, herr := suite(ctx)
			if herr != nil || h.SpineOldest == nil {
				return health.OK(fmt.Sprintf("keeping %dd", days))
			}
			if hoursSince(h.SpineOldest) > float64(days)*24*1.2 { // 20% grace absorbs the daily prune cadence
				return health.Warn(fmt.Sprintf("oldest event %s, keeping %dd", agoText(h.SpineOldest), days),
					"Events older than the retention window are still stored - the daily prune may not be running")
			}
			return health.OK(fmt.Sprintf("oldest event %s, keeping %dd", agoText(h.SpineOldest), days))
		}})

	r.Register(health.Check{Group: "signals", Key: "queue", Label: "Ingest queue", Order: 5,
		Run: func(ctx context.Context) health.Result {
			h, err := suite(ctx)
			if err != nil {
				return health.Bad("unreachable", err.Error())
			}
			q := h.Queue
			if !q.Configured {
				return health.Off("not configured", "Events are written straight to the engine. That is the default - a queue only adds durability across restarts")
			}
			if q.Error != "" {
				return health.Bad("unreachable", q.Error+" - configured but not responding, so events cannot be buffered")
			}
			// The dangerous one: a queue with no consumer accepts writes and sheds them
			// at the stream's MaxAge, silently, a day later
			if !q.HasConsumer {
				return health.Bad(fmt.Sprintf("no consumer, %d in stream", q.StreamMessages),
					"Nothing is draining the queue. Events are accepted and will be discarded after 24h - start the signals worker")
			}
			if q.Pending > 10_000 {
				return health.Warn(fmt.Sprintf("%d waiting, %d in stream", q.Pending, q.StreamMessages),
					"The consumer is falling behind. Check the worker and the engine it writes to")
			}
			if q.Redelivering > 0 {
				return health.Warn(fmt.Sprintf("%d un-acked", q.Redelivering),
					"Delivered but not accepted - the engine is likely rejecting writes")
			}
			return health.OK(fmt.Sprintf("draining, %d waiting", q.Pending))
		}})

	r.Register(health.Check{Group: "signals", Key: "erasures", Label: "Erasures awaiting sweep", Order: 6,
		Run: func(ctx context.Context) health.Result {
			h, err := suite(ctx)
			if err != nil {
				return health.Bad("unreachable", err.Error())
			}
			if h.ErasuresError != "" {
				return health.Bad("unreadable", h.ErasuresError)
			}
			if h.PendingErasures > 0 {
				return health.Warn(fmt.Sprintf("%d queued", h.PendingErasures),
					"Queued deletions are applied by the daily sweep. Normal for a few hours on the columnar engine; on Postgres this should stay at zero")
			}
			return health.OK("none queued")
		}})
}

// Liveness says dependencies answer, readiness says the plugin is configured - a
// gateway nobody set up legitimately fails readiness, so that is a note not a fault
func registerPluginChecks(r *health.Registry) {
	r.RegisterGroup(health.Group{Key: "plugins", Label: "Plugins", Order: 6})

	r.Register(health.Check{Group: "plugins", Key: "liveness", Label: "Dependencies", Order: 1,
		Run: func(ctx context.Context) health.Result {
			live := pluginhost.HealthStatuses(ctx)
			if len(live) == 0 {
				return health.Off("no probes registered", "")
			}
			// An installed gateway registers keyless and its liveness probe builds a client from config, so an unused one fails BY CONSTRUCTION - counting that as broken makes a store that simply doesn't take that gateway look on fire
			unconfigured := map[string]bool{}
			for _, p := range pluginhost.ReadinessStatuses(ctx) {
				if !p.OK {
					unconfigured[p.Plugin] = true
				}
			}
			var failed []string
			skipped := 0
			for _, p := range live {
				switch {
				case p.OK:
				case unconfigured[p.Plugin]:
					skipped++
				default:
					failed = append(failed, p.Name)
				}
			}
			checked := len(live) - skipped
			if len(failed) > 0 {
				return health.Bad(fmt.Sprintf("%d of %d configured plugins failing", len(failed), checked), strings.Join(failed, ", "))
			}
			if checked == 0 {
				return health.Off("no configured plugins", "")
			}
			return health.OK(fmt.Sprintf("%d probes passing", checked))
		}})

	// Not-configured is a choice rather than a fault, so it reports OFF and can't escalate - a store using one gateway would otherwise sit at WARN forever
	r.Register(health.Check{Group: "plugins", Key: "readiness", Label: "Configuration", Order: 2,
		Run: func(ctx context.Context) health.Result {
			probes := pluginhost.ReadinessStatuses(ctx)
			if len(probes) == 0 {
				return health.Off("no probes registered", "")
			}
			notReady := 0
			for _, p := range probes {
				if !p.OK {
					notReady++
				}
			}
			ready := len(probes) - notReady
			switch {
			case ready == 0:
				return health.Off(fmt.Sprintf("none of %d configured", len(probes)), "")
			case notReady > 0:
				return health.OK(fmt.Sprintf("%d of %d configured", ready, len(probes)))
			}
			return health.OK(fmt.Sprintf("%d probes ready", ready))
		}})
}

func registerBackupChecks(r *health.Registry, d healthDeps) {
	r.RegisterGroup(health.Group{Key: "backups", Label: "Backups", Order: 7})

	r.Register(health.Check{Group: "backups", Key: "last", Label: "Last backup", Order: 1,
		Run: func(ctx context.Context) health.Result {
			if !d.Backup.Enabled() {
				return health.Off("not configured", "BACKUP_S3_BUCKET is unset - nothing is being backed up")
			}
			keys, err := d.Backup.List(ctx)
			if err != nil {
				return health.Bad("unreadable", err.Error())
			}
			if len(keys) == 0 {
				return health.Bad("never run", "The destination is configured but holds no dump")
			}
			// One run writes a commerce dump AND a signals one under one stamp, so count stamps not objects - else "keeping 7" reads 14
			var newest *time.Time
			stamps := map[time.Time]struct{}{}
			for _, k := range keys {
				at, ok := backup.KeyTimestamp(k)
				if !ok {
					continue
				}
				stamps[at] = struct{}{}
				if newest == nil || at.After(*newest) {
					at := at
					newest = &at
				}
			}
			if newest == nil {
				return health.OK(fmt.Sprintf("%d dumps stored", len(keys)))
			}
			detail := fmt.Sprintf("%s (%d kept)", agoText(newest), len(stamps))
			switch {
			case hoursSince(newest) > 48:
				return health.Bad(detail, "No dump in over two days - check the backup job in Scheduled jobs")
			case hoursSince(newest) > 26:
				return health.Warn(detail, "The daily backup looks like it missed a run")
			}
			return health.OK(detail)
		}})

	r.Register(health.Check{Group: "backups", Key: "encryption", Label: "Encryption at rest", Order: 2,
		Run: func(ctx context.Context) health.Result {
			if !d.Backup.Enabled() {
				return health.Off("not configured", "")
			}
			if !d.Backup.Encrypted() {
				return health.Warn("dumps are unencrypted",
					"A dump is a full copy of every customer record. Set BACKUP_AGE_RECIPIENT to encrypt them at rest")
			}
			return health.OK("age-encrypted")
		}})
}

func hoursSince(t *time.Time) float64 {
	if t == nil {
		return math_Inf
	}
	return time.Since(*t).Hours()
}

const math_Inf = 1e9 // a sentinel, so "never" compares greater than every threshold without importing math for one value

func olderForward(a, b *time.Time) bool {
	if a == nil {
		return true
	}
	if b == nil {
		return false
	}
	return a.Before(*b)
}

func agoText(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return roundDur(time.Since(*t)) + " ago"
}

func roundDur(d time.Duration) string {
	switch {
	case d < 90*time.Second:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < 90*time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
