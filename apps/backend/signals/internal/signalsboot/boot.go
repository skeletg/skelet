// Package signalsboot assembles the Signals spine stack shared by cmd/server, cmd/signals and cmd/ingest, so the order the Recorder needs (enabled -> read pools -> engine -> queue wrap) is right by construction instead of re-derived per main
// First extracted slice of cmd/server's wiring - the Signals half was the piece duplicated three ways with order-sensitive landmines; the commerce assembly stays in cmd/server until a second consumer exists
package signalsboot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics/spinech"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics/spinequeue"
	"github.com/skeletg/skelet/apps/backend/signals/internal/archivefile"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Settings is the slice of the store's settings the retention and trait bodies read, declared here so the package depends on the shape rather than on whoever holds them
// A nil channel asks for the store-wide value
type Settings interface {
	Int(ctx context.Context, path string, channelID *uuid.UUID) (int64, error)
	String(ctx context.Context, path string, channelID *uuid.UUID) (string, error)
}

// Options carries the per-binary database handles - everything env-shaped (SIGNALS_CH_URL, SIGNALS_QUEUE_URL) is read here
// No switch here - SIGNALS_ENABLED is core's, a deployed suite is on
type Options struct {
	Queries   *signalsdb.Queries // the signals database
	WrapQueue bool               // producers wrap appends in the JetStream publisher (on disk before the request returns); the worker leaves the spine bare and CONSUMES the stream
	// Where retention archives aged data before deleting it - nil leaves the sweep with no sink, which fails it closed
	ArchiveStore  archivefile.ObjectStore
	ArchivePrefix string
}

type Stack struct {
	Recorder *analytics.Recorder
	Spine    *spinech.Store              // the engine itself, for the backup job
	Queue    *spinequeue.Queue           // non-nil when SIGNALS_QUEUE_URL is set
	CHPing   func(context.Context) error // the spine's liveness, for health endpoints
	cleanups []func()
}

func Build(ctx context.Context, o Options) (*Stack, error) {
	spine, spineCleanup, err := spinech.FromEnv(ctx)
	if err != nil {
		return nil, fmt.Errorf("clickhouse spine: %w", err)
	}
	rec := analytics.NewRecorder(o.Queries, spine)
	st := &Stack{Recorder: rec, Spine: spine, CHPing: spine.Ping, cleanups: []func(){spineCleanup}}
	// retention_action=archive writes aged data out before deleting it - unset, the sweep fails closed and deletes nothing
	if o.ArchiveStore != nil {
		rec.SetArchiveSink(archivefile.NewStorageSink(o.ArchiveStore, o.ArchivePrefix))
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

// TraitComputeJob is the trait-compute body (RFM / lifecycle / engagement), run when core's scheduler asks for it
// Lifecycle windows are per-channel so staging matches the catalog's repurchase cadence, falling back to the registered defaults
func TraitComputeJob(rec *analytics.Recorder, cfgSvc Settings) func(context.Context) error {
	return func(ctx context.Context) error {
		chs, err := rec.RosterChannels(ctx)
		if err != nil {
			return err
		}
		now := time.Now()
		for _, chID := range chs {
			cfg := func(key string, def int) int {
				if v, err := cfgSvc.Int(ctx, key, &chID); err == nil && v > 0 {
					return int(v)
				}
				return def
			}
			lc := analytics.LifecycleWindows{
				NewDays:    cfg("signals/synapse/lifecycle_new_days", 30),
				ActiveDays: cfg("signals/synapse/lifecycle_active_days", 90),
				AtRiskDays: cfg("signals/synapse/lifecycle_at_risk_days", 180),
			}
			if _, err := rec.ComputeTraits(ctx, chID, now, lc); err != nil {
				return err
			}
		}
		return nil
	}
}
