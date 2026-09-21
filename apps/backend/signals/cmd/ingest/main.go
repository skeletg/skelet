// Command ingest serves the public /t surface (storefront tracker beacons and SDK reads) as its own
// horizontally-scalable process - the same handler cmd/signals mounts, on a pool sized for beacon spikes
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/envcheck"
	"github.com/skeletg/skelet/apps/backend/core/lib/httpx"
	"github.com/skeletg/skelet/apps/backend/core/lib/observability"
	"github.com/skeletg/skelet/apps/backend/signals/internal/boot"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ingest tier exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	applog.Setup(os.Getenv("APP_ENV"))

	// Env validation before anything opens a connection, same order as the other entrypoints
	if err := envcheck.Report(envcheck.Production(os.Getenv,
		envcheck.JWTSecret, envcheck.TrustedProxies, envcheck.SignalsDatabase,
		envcheck.RateLimiting, // the beacon endpoint is public and unauthenticated
	)); err != nil {
		return err
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

	// One pool, and a producer's stack: beacons land in JetStream before the 204 when a queue is
	// configured, and the cmd/signals consumer drains it
	var d boot.IngestDeps
	app := fx.New(
		fx.Provide(boot.LoadIngest),
		boot.IngestPools,
		boot.Services,
		boot.Pulse, // for /e - the hit lands in the send log and starts the engagement flows
		boot.Wiring,
		boot.Beacon,
		boot.IngestFallback,
		fx.Supply(boot.RootContext{Context: ctx}, boot.QueueProducer),
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

	var wg sync.WaitGroup
	wg.Go(func() { boot.SuperviseLoop(ctx, d.Rollup.Run) })

	srv := &http.Server{
		Addr:              ":" + boot.EnvOr("INGEST_PORT", "8091"),
		Handler:           httpx.Recover(httpx.RequestID(d.Mux)), // panic -> structured 500 with request_id instead of an aborted connection
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
