// Package signalsboot assembles the Signals spine stack shared by cmd/server, cmd/signals and cmd/ingest, so the order the Recorder needs (enabled -> read pools -> engine -> queue wrap) is right by construction instead of re-derived per main
// First extracted slice of cmd/server's wiring - the Signals half was the piece duplicated three ways with order-sensitive landmines; the commerce assembly stays in cmd/server until a second consumer exists
package signalsboot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/analytics/spinech"
	"github.com/skeletg/skelet/apps/backend/internal/analytics/spinequeue"
	"github.com/skeletg/skelet/apps/backend/internal/backup"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

// Options carries the per-binary database handles - everything env-shaped (SIGNALS_ENABLED, SIGNALS_ENGINE, SIGNALS_CH_URL, SIGNALS_QUEUE_URL) is read here
type Options struct {
	Commerce     *db.Queries        // traits/email_sends/identity orchestration handle
	CommerceRead *db.Queries        // optional - heavy commerce aggregations (replica when configured)
	Spine        *signalsdb.Queries // spine + Lens write handle
	SpineRead    *signalsdb.Queries // optional - spine read-pool twin
	WrapQueue    bool               // producers wrap appends in the JetStream publisher (on disk before the request returns); the worker leaves the spine bare and CONSUMES the stream
}

type Stack struct {
	Recorder *analytics.Recorder
	Queue    *spinequeue.Queue           // non-nil when SIGNALS_QUEUE_URL is set
	Enabled  bool                        // SIGNALS_ENABLED (the Recorder already no-ops its writes when off)
	CHPing   func(context.Context) error // non-nil under the ClickHouse engine, for health endpoints
	cleanups []func()
}

func Build(ctx context.Context, o Options) (*Stack, error) {
	rec := analytics.NewRecorder(o.Commerce, o.Spine)
	st := &Stack{Recorder: rec, Enabled: envBool("SIGNALS_ENABLED", true)}
	rec.SetEnabled(st.Enabled)
	if !st.Enabled {
		slog.Info("signals disabled via SIGNALS_ENABLED=false")
	}
	if o.CommerceRead != nil {
		rec.SetReadQueries(o.CommerceRead)
	}
	if o.SpineRead != nil {
		rec.SetSpineReadQueries(o.SpineRead)
	}
	// engine selection runs even when disabled ON PURPOSE - GDPR erasure and retention must keep targeting whichever store holds the data
	chStore, chCleanup, err := spinech.SelectFromEnv(ctx)
	if err != nil {
		return nil, fmt.Errorf("clickhouse spine engine: %w", err)
	}
	if chStore != nil {
		rec.SetSpine(chStore)
		st.cleanups = append(st.cleanups, chCleanup)
		if p, ok := chStore.(interface{ Ping(context.Context) error }); ok {
			st.CHPing = p.Ping
		}
		slog.Info("signals spine engine", "engine", "clickhouse")
	} else {
		slog.Info("signals spine engine", "engine", "postgres")
	}
	// retention archive destination (retention_action=archive) - a broken destination only warns, because the sweep fails closed without a sink rather than deleting anything
	if store, prefix, err := backup.ArchiveStorage(ctx); err != nil {
		slog.Warn("retention archive destination unavailable", "err", err)
	} else if store != nil {
		rec.SetArchiveSink(archiveSink{store: store, prefix: prefix})
	}
	if qURL := os.Getenv("SIGNALS_QUEUE_URL"); qURL != "" {
		q, err := spinequeue.New(ctx, qURL)
		if err != nil {
			st.Close()
			return nil, fmt.Errorf("signals ingest queue: %w", err)
		}
		st.Queue = q
		st.cleanups = append(st.cleanups, q.Close)
		if o.WrapQueue {
			rec.WrapSpine(q.WrapSpine) // LAST - SetSpine/SetSpineReadQueries replace the store wholesale
			slog.Info("signals ingest queue enabled", "url", qURL)
		}
	}
	return st, nil
}

// Close releases in reverse build order: the queue connection first, then the ClickHouse store (which drains its ingest buffer) - call before the DB pools close
func (s *Stack) Close() {
	for _, v := range slices.Backward(s.cleanups) {
		v()
	}
}

// TraitComputeJob is the shared trait-compute body (RFM / lifecycle / engagement), owned by cmd/signals under SIGNALS_JOBS_EXTERNAL and by cmd/server otherwise
// Lifecycle windows are per-channel so staging matches the catalog's repurchase cadence (tire shop >> consumables), falling back to the registered defaults
func TraitComputeJob(rec *analytics.Recorder, channels *channel.Service, cfgSvc *config.Service) func(context.Context) error {
	return func(ctx context.Context) error {
		chs, err := channels.List(ctx)
		if err != nil {
			return err
		}
		now := time.Now()
		for _, ch := range chs {
			sc := config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}
			cfg := func(key string, def int) int {
				if v, err := cfgSvc.GetInt(ctx, key, sc); err == nil && v > 0 {
					return int(v)
				}
				return def
			}
			lc := analytics.LifecycleWindows{
				NewDays:    cfg("pulse/synapse/lifecycle_new_days", 30),
				ActiveDays: cfg("pulse/synapse/lifecycle_active_days", 90),
				AtRiskDays: cfg("pulse/synapse/lifecycle_at_risk_days", 180),
			}
			if _, err := rec.ComputeTraits(ctx, ch.ID, now, lc); err != nil {
				return err
			}
		}
		return nil
	}
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
