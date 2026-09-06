package wiring

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	dbconn "github.com/skeletg/skelet/apps/backend/internal/db"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/otelx"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

type ( // all four pools are *sql.DB so the graph needs distinct types; the commerce primary keeps the untagged slot
	ReadPool        struct{ *sql.DB }
	SignalsPool     struct{ *sql.DB }
	SignalsReadPool struct{ *sql.DB }
)

type ( // same problem, same fix, for the two sqlc query sets per datastore
	ReadQueries        struct{ *db.Queries }
	SignalsQueries     struct{ *signalsdb.Queries }
	SignalsReadQueries struct{ *signalsdb.Queries }
)

// Pools is the datastore layer alone - shareable by every entrypoint, because nothing in it is specific to serving HTTP
var Pools = fx.Module("pools",
	fx.Provide(
		newQueryTracer,
		newPrimaryPool,
		newReadPool,
		newSignalsPool,
		newSignalsReadPool,
		newQueries,
		newReadQueries,
		newSignalsQueries,
		newSignalsReadQueries,
		newTxManager,
	),
)

var Cache = fx.Module("cache", fx.Provide(newValkey, newValkeyCache)) // Valkey on its own - a Signals worker needs no cache, rate limiter or preview store, so it must not open a connection it never reads

type CacheRedis struct{ *redis.Client } // evictable Valkey for caches/limiters/APQ - kept off the noeviction primary since these namespaces are anonymous-writable and unbounded - a flood evicts here instead of failing auth-token writes

// Registries is the process-global bootstrap - deliberately NOT in Pools, since a worker must not replay plugin registrations or sync permissions
var Registries = fx.Module("registries",
	fx.Invoke( // fx runs invokes in registration order, and that matters: plugins install before anything validates the registries they contribute to, and permissions validate before they sync
		bootstrapRegistries,
		applyTrustedProxies,
		syncPermissions,
	),
)

func newQueryTracer(cfg Config) pgx.QueryTracer { // dev wraps the OTel tracer in a counter so X-DB-Queries can report statements per request
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

func newPrimaryPool(lc fx.Lifecycle, cfg Config, tracer pgx.QueryTracer) (*sql.DB, error) {
	return openPool(lc, "primary", cfg.DatabaseURL, tracer, cfg.MaxConns)
}

// Bulkhead so heavy Signals aggregations can't starve the primary's budget, and so DATABASE_REPLICA_URL can point them at a replica without code changes - writes stay on the primary
func newReadPool(lc fx.Lifecycle, cfg Config, tracer pgx.QueryTracer) (ReadPool, error) {
	conn, err := openPool(lc, "analytics read", cfg.ReplicaURL, tracer, cfg.AnalyticsMaxConns)
	return ReadPool{conn}, err
}

func newSignalsPool(lc fx.Lifecycle, cfg Config, tracer pgx.QueryTracer) (SignalsPool, error) { // same bulkhead on the Signals datastore - ingest writes can't drain the budget checkout needs
	conn, err := openPool(lc, "signals write", cfg.SignalsURL, tracer, cfg.SignalsMaxConns)
	return SignalsPool{conn}, err
}

func newSignalsReadPool(lc fx.Lifecycle, cfg Config, tracer pgx.QueryTracer) (SignalsReadPool, error) {
	conn, err := openPool(lc, "signals read", cfg.SignalsReadURL, tracer, cfg.SignalsReadMaxConns)
	return SignalsReadPool{conn}, err
}

// OPTIONAL - empty VALKEY_URL runs on Postgres alone: weaker caches, per-process limits, no draft previews - a malformed URL is still fatal, that's a typo rather than a deployment choice
func newValkey(lc fx.Lifecycle, cfg Config) (*redis.Client, error) {
	if cfg.ValkeyURL == "" {
		slog.Warn("VALKEY_URL is unset: running Postgres-only",
			"caches", "disabled",
			"rate_limits", "per-process",
			"auth_tokens", "postgres",
			"draft_previews", "unavailable")
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

// empty or same-as-primary VALKEY_CACHE_URL shares the primary's pool with no extra close hook - a distinct URL opens its own client that prod runs as a second allkeys-lru instance
func newValkeyCache(lc fx.Lifecycle, cfg Config, primary *redis.Client) (CacheRedis, error) {
	url := strings.TrimSpace(cfg.ValkeyCacheURL)
	if url == "" || url == cfg.ValkeyURL {
		return CacheRedis{primary}, nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return CacheRedis{}, fmt.Errorf("parse valkey cache url: %w", err)
	}
	client := redis.NewClient(opt)
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return client.Close() }})
	if err := otelx.InstrumentRedis(client); err != nil {
		slog.Warn("instrument redis (cache)", "err", err)
	}
	slog.Info("VALKEY_CACHE_URL set: caches and rate limiters use a separate Valkey from the auth-token primary")
	return CacheRedis{client}, nil
}

func newQueries(conn *sql.DB) *db.Queries { return db.New(conn) }
func newTxManager(conn *sql.DB) *tx.Manager {
	return tx.New(conn)
}

func newReadQueries(p ReadPool) ReadQueries { return ReadQueries{db.New(p.DB)} }
func newSignalsQueries(p SignalsPool) SignalsQueries {
	return SignalsQueries{signalsdb.New(p.DB)}
}
func newSignalsReadQueries(p SignalsReadPool) SignalsReadQueries {
	return SignalsReadQueries{signalsdb.New(p.DB)}
}
