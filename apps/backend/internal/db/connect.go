package db

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

type Option func(*config)

type config struct {
	tracer   pgx.QueryTracer
	maxConns int // 0 = default (25)
}

// Attaches a pgx-level tracer to every connection Connect opens, which is how otelpgx gets a span per SQL execution - a nil tracer is silently ignored so callers needn't branch on whether OTel is initialised
func WithQueryTracer(t pgx.QueryTracer) Option {
	return func(c *config) { c.tracer = t }
}

// Sizes the pool independently, e.g. a smaller dedicated analytics-read pool alongside the larger primary so heavy reads can't exhaust the primary's budget - n <= 0 keeps the default of 25
func WithMaxConns(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.maxConns = n
		}
	}
}

// Opens a postgres connection through the pgx driver, retrying Ping with jittered backoff for up to 30s so transient unavailability - a rolling upgrade, failover, container start ordering - doesn't crash the process at boot
// Use WithQueryTracer for OTel pgx instrumentation; without options this behaves exactly like sql.Open on the pgx stdlib driver
func Connect(databaseURL string, opts ...Option) (*sql.DB, error) {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}

	var conn *sql.DB
	if cfg.tracer == nil {
		var err error
		conn, err = sql.Open("pgx", databaseURL)
		if err != nil {
			return nil, fmt.Errorf("open db: %w", err)
		}
	} else {
		// Attaching a Tracer means going through ParseConfig and stdlib.OpenDB rather than sql.Open - the DSN parsing is the same one pgx applies internally, so behavior is unchanged modulo the tracer
		pgxCfg, err := pgx.ParseConfig(databaseURL)
		if err != nil {
			return nil, fmt.Errorf("parse db config: %w", err)
		}
		pgxCfg.Tracer = cfg.tracer
		conn = stdlib.OpenDB(*pgxCfg)
	}

	// Bound the pool so a traffic spike can't exhaust Postgres connections, since a default sql.DB allows unlimited open conns
	maxConns := cfg.maxConns
	if maxConns <= 0 {
		maxConns = 25
	}
	conn.SetMaxOpenConns(maxConns)
	conn.SetMaxIdleConns(maxConns)
	conn.SetConnMaxLifetime(5 * time.Minute)
	conn.SetConnMaxIdleTime(5 * time.Minute)

	deadline := time.Now().Add(30 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := conn.PingContext(ctx)
		cancel()
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("ping db: %w", err)
		}
		time.Sleep(300*time.Millisecond + time.Duration(rand.Int63n(int64(400*time.Millisecond))))
	}
}
