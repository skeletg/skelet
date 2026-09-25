package boot

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Named wrappers, so the graph cannot hand a write to the read pool or the other way round
type (
	SignalsPool     struct{ *sql.DB }
	SignalsReadPool struct{ *sql.DB }
	Queries         struct{ *signalsdb.Queries }
	// ReadQueries is nil on the beacon tier, which opens no read pool - reads fall back to the primary
	ReadQueries struct{ *signalsdb.Queries }
)

// Pools is the datastore layer for cmd/signals: the write pool, a read pool the heavy reports run on,
// and the optional Valkey
var Pools = fx.Module("pools",
	fx.Provide(
		newQueryTracer,
		newSignalsPool,
		newSignalsReadPool,
		newQueries,
		newReadQueries,
		newTxManager,
		newValkey,
		newLimitsValkey,
		newRevocations,
	),
)

// IngestPools is the beacon tier's: one pool only, since this tier absorbs beacon spikes and a second
// pool per replica buys nothing - it sets no read handles on purpose, so reads fall back to the primary
var IngestPools = fx.Module("ingestpools",
	fx.Provide(
		newQueryTracer,
		newSignalsPool,
		newQueries,
		func() ReadQueries { return ReadQueries{} },
		newTxManager,
		newValkey,
		newLimitsValkey,
	),
)

// LimitsValkey holds the rate-limit counters, kept off the evictable cache when VALKEY_LIMITS_URL names its own instance
type LimitsValkey struct{ *redis.Client }

func newLimitsValkey(lc fx.Lifecycle, cfg Config, vk *redis.Client) (LimitsValkey, error) {
	if cfg.ValkeyLimitsURL == "" || cfg.ValkeyLimitsURL == cfg.ValkeyURL {
		return LimitsValkey{vk}, nil
	}
	opt, err := redis.ParseURL(cfg.ValkeyLimitsURL)
	if err != nil {
		return LimitsValkey{}, fmt.Errorf("parse valkey limits url: %w", err)
	}
	client := redis.NewClient(opt)
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return client.Close() }})
	return LimitsValkey{client}, nil
}

// The killed-session list core writes on the primary Valkey - reused when the cache client already points
// there, opened separately when the two are split, and a no-op reader with no primary at all
func newRevocations(lc fx.Lifecycle, cfg Config, vk *redis.Client) (auth.Revocations, error) {
	const ttl = 16 * time.Minute // core's write side sets it, this side only reads
	switch {
	case cfg.PrimaryValkeyURL == "":
		return cache.NewRevocations(nil, ttl), nil
	case cfg.PrimaryValkeyURL == cfg.ValkeyURL && vk != nil:
		return cache.NewRevocations(vk, ttl), nil
	}
	opt, err := redis.ParseURL(cfg.PrimaryValkeyURL)
	if err != nil {
		return nil, fmt.Errorf("parse primary valkey url: %w", err)
	}
	client := redis.NewClient(opt)
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return client.Close() }})
	return cache.NewRevocations(client, ttl), nil
}

func newQueryTracer(cfg Config) pgx.QueryTracer { // dev wraps the OTel tracer in a counter, the same way core does
	tracer := otelx.PGXTracer()
	if cfg.IsDev() {
		tracer = dbconn.CountingTracer(tracer)
	}
	return tracer
}

func openPool(lc fx.Lifecycle, label, dsn string, tracer pgx.QueryTracer, maxConns int) (*sql.DB, error) {
	conn, err := dbconn.Connect(dsn, dbconn.WithQueryTracer(tracer), dbconn.WithMaxConns(maxConns))
	if err != nil {
		return nil, fmt.Errorf("connect to %s database: %w", label, err)
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return conn.Close() }})
	return conn, nil
}

func newSignalsPool(lc fx.Lifecycle, cfg Config, tracer pgx.QueryTracer) (SignalsPool, error) {
	conn, err := openPool(lc, "signals write", cfg.SignalsURL, tracer, cfg.SignalsMaxConns)
	return SignalsPool{conn}, err
}

// Bulkhead so the heavy aggregations cannot starve the writes, and so SIGNALS_REPLICA_URL can point
// them at a replica without code changes
func newSignalsReadPool(lc fx.Lifecycle, cfg Config, tracer pgx.QueryTracer) (SignalsReadPool, error) {
	conn, err := openPool(lc, "signals read", cfg.SignalsReadURL, tracer, cfg.SignalsReadMaxConns)
	return SignalsReadPool{conn}, err
}

func newQueries(p SignalsPool) Queries             { return Queries{signalsdb.New(p.DB)} }
func newReadQueries(p SignalsReadPool) ReadQueries { return ReadQueries{signalsdb.New(p.DB)} }
func newTxManager(p SignalsPool) *txrunner.Manager { return txrunner.New(p.DB) }

// OPTIONAL - unset runs on Postgres alone: per-process rate limits and uncached settings reads
// A malformed URL is still fatal, that's a typo rather than a deployment choice
func newValkey(lc fx.Lifecycle, cfg Config) (*redis.Client, error) {
	if cfg.ValkeyURL == "" {
		slog.Warn("VALKEY_URL is unset: running Postgres-only", "rate_limits", "per-process", "settings_cache", "disabled")
		return nil, nil
	}
	opt, err := redis.ParseURL(cfg.ValkeyURL)
	if err != nil {
		return nil, fmt.Errorf("parse valkey url: %w", err)
	}
	client := redis.NewClient(opt)
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return client.Close() }})
	if err := otelx.InstrumentRedis(client); err != nil {
		slog.Warn("instrument redis", "err", err)
	}
	return client, nil
}
