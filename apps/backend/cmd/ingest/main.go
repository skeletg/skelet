// Command ingest serves the public /t surface (storefront tracker beacons and
// SDK reads) as its own horizontally-scalable process

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/skeletg/skelet/apps/backend/internal/applog"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/envcheck"
	"github.com/skeletg/skelet/apps/backend/internal/httpx"
	"github.com/skeletg/skelet/apps/backend/internal/ingesthttp"
	"github.com/skeletg/skelet/apps/backend/internal/observability"
	"github.com/skeletg/skelet/apps/backend/internal/ratelimit"
	"github.com/skeletg/skelet/apps/backend/internal/wiring"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ingest tier exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	applog.Setup(os.Getenv("APP_ENV"))

	// Env validation before anything opens a connection, same order as cmd/server
	if err := envcheck.Report(envcheck.Production(os.Getenv,
		envcheck.JWTSecret, envcheck.TrustedProxies, envcheck.ConfigEncryptionKey, envcheck.Database,
		envcheck.RateLimiting, // the beacon endpoint is public and unauthenticated
	)); err != nil {
		return err
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return errors.New("JWT_SECRET is required - /t verifies customer sessions with it")
	}

	if err := auth.SetTrustedProxies(strings.Split(os.Getenv("TRUSTED_PROXIES"), ",")); err != nil {
		return fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	telemetry := observability.Init(ctx, os.Getenv("APP_ENV"))
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(sctx); err != nil {
			slog.Warn("observability shutdown", "err", err)
		}
	}()

	// Two pools only, and no Registries or Platform wiring - see wiring.IngestPools
	var d wiring.IngestDeps
	app := fx.New(
		fx.Provide(wiring.LoadIngestConfig),
		wiring.IngestPools,
		wiring.Cache,
		wiring.Settings,
		wiring.IngestSignals,
		wiring.IngestWiring,
		fx.Supply(wiring.RootContext{Context: ctx}),
		fx.Populate(&d),
		fx.WithLogger(func() fxevent.Logger {
			l := &fxevent.SlogLogger{Logger: slog.Default()}
			l.UseLogLevel(slog.LevelDebug)
			return l
		}),
	)
	if err := app.Start(ctx); err != nil {
		return err
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.Stop(sctx); err != nil {
			slog.Warn("wiring shutdown", "err", err)
		}
	}()

	primaryConn := d.DB
	valkeyClient := d.Cache.Client
	stack := d.Stack
	chPing := stack.CHPing
	configSvc := d.ConfigSvc
	lensSvc := d.Lens

	var wg sync.WaitGroup
	wg.Go(func() { wiring.SuperviseLoop(ctx, d.Rollup.Run) })
	channelSvc := d.ChannelSvc
	experimentSvc := d.Experiment
	segmentSvc := d.Segment

	geoResolver := wiring.NewGeoResolver()

	// forms/back-in-stock proxy through to the app (they need its subscriber
	// graph); unset -> those two paths 404 and the SDKs fail soft
	var fallback http.Handler
	if raw := os.Getenv("INGEST_FALLBACK_URL"); raw != "" {
		target, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("parse INGEST_FALLBACK_URL: %w", err)
		}
		fallback = httputil.NewSingleHostReverseProxy(target)
	} else {
		slog.Warn("INGEST_FALLBACK_URL unset - /t/forms* and /t/back-in-stock will 404 here, route them to the app")
	}

	mux := http.NewServeMux()
	wiring.RegisterPprof(mux)
	mux.Handle("/t/", ingesthttp.New(ingesthttp.Options{
		Channels: channelSvc, Lens: lensSvc, Experiments: experimentSvc, Segments: segmentSvc,
		Config: configSvc, Geo: geoResolver, Rollup: d.Rollup,
		Limiter:   ratelimit.NewRedis(valkeyClient, wiring.RateLimitPerMin("INGEST_RATE_LIMIT_PER_MIN", 300), time.Minute),
		JWTSecret: jwtSecret, SignalsEnabled: stack.Enabled,
		Fallback: fallback,
	}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := primaryConn.PingContext(r.Context()); err != nil {
			http.Error(w, "commerce db unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := d.SignalsPool.PingContext(r.Context()); err != nil {
			http.Error(w, "signals db unavailable", http.StatusServiceUnavailable)
			return
		}
		if chPing != nil {
			if err := chPing(r.Context()); err != nil {
				http.Error(w, "clickhouse unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              ":" + envDefault("INGEST_PORT", "8091"),
		Handler:           httpx.Recover(httpx.RequestID(mux)), // panic -> structured 500 with request_id instead of an aborted connection
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       1 * time.Minute, // beacon-sized bodies - a request holding a connection past this is a stalled client
		WriteTimeout:      1 * time.Minute,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	slog.Info("ingest tier started", "addr", srv.Addr)
	var listenErr error
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		listenErr = err // still fall through to the wait - a listener that never bound leaves the rollup running, and the deferred app.Stop closes the pool under its final flush
		stop()          // nothing else will cancel ctx on this path
	}
	wg.Wait()
	slog.Info("ingest tier stopped")
	return listenErr
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
