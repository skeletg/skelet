// Command signals runs the Signals suite: the wire contract core calls, the /t and /e surfaces core proxies here, and the upkeep core's scheduler asks for
// It opens the suite's own database and nothing of core's, so an analytics overload cannot take checkout down
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/envcheck"
	"github.com/skeletg/skelet/apps/backend/core/lib/httpx"
	"github.com/skeletg/skelet/apps/backend/core/lib/observability"
	"github.com/skeletg/skelet/apps/backend/signals/internal/boot"
)

func main() {
	if err := run(); err != nil {
		slog.Error("signals exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	applog.Setup(os.Getenv("APP_ENV"))

	// The contract verifies core's calls with the service secret, /t the customer sessions with JWT_SECRET
	if err := envcheck.Report(envcheck.Production(os.Getenv,
		envcheck.JWTSecret, envcheck.SignalsServiceSecret, envcheck.SignalsDatabase, envcheck.RateLimiting,
	)); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM) // SIGTERM/SIGINT cancels ctx - drives graceful shutdown of the loops and the servers so in-flight prunes and forwards finish before the pools close
	defer stop()

	telemetry := observability.Init(ctx, os.Getenv("APP_ENV")) // Traces/metrics when OTEL_* is set, no-op otherwise - as a separate process this is how you follow an event across the service boundary
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(sctx); err != nil {
			slog.Warn("observability shutdown", "err", err)
		}
	}()

	// QueueConsumer is the one real difference from the beacon tier: this process drains the stream, so
	// its own spine handle must stay the bare engine rather than wrapping into it
	var d boot.Deps
	app := fx.New(
		fx.Provide(boot.Load),
		boot.Pools,
		boot.Services,
		boot.Pulse,
		boot.Wiring,
		boot.Beacon,
		boot.Contract,
		fx.Supply(boot.RootContext{Context: ctx}, boot.QueueConsumer),
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

	// Retention, trait compute and destination forwarding arrive on the contract - core's scheduler
	// owns their cadence, their run history and the operator's run-now button
	var wg sync.WaitGroup
	if queue := d.Stack.Queue; queue != nil { // this process never wraps its spine, so the recorder's store is the engine itself
		wg.Go(func() {
			// SuperviseLoop keeps a panicking consumer from killing the process - a clean Consume error stays terminal
			boot.SuperviseLoop(ctx, func(ctx context.Context) {
				if err := queue.Consume(ctx, d.Stack.Recorder.SpineStore()); err != nil {
					slog.Error("signals queue consumer", "err", err)
				}
			})
		})
		slog.Info("signals ingest queue consumer started")
	}
	wg.Go(func() { boot.SuperviseLoop(ctx, d.Rollup.Run) })

	// The wire contract, on its own listener - grpc needs one
	grpcAddr := ":" + boot.EnvOr("SIGNALS_GRPC_PORT", "8092")
	grpcLn, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("listen grpc: %w", err)
	}
	wg.Go(func() {
		if err := d.GRPC.Serve(grpcLn); err != nil {
			slog.Error("signals grpc server", "err", err)
		}
	})
	go func() {
		<-ctx.Done()
		d.GRPC.GracefulStop()
	}()
	slog.Info("signals grpc listening", "addr", grpcAddr)

	srv := &http.Server{
		Addr:              ":" + boot.EnvOr("SIGNALS_PORT", "8090"),
		Handler:           httpx.Recover(httpx.RequestID(d.Mux)), // panic -> structured 500 with request_id instead of an aborted connection
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute, // event batches and rollup reads, nothing long-streaming
		WriteTimeout:      2 * time.Minute,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	slog.Info("signals started", "addr", srv.Addr)
	var listenErr error
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		listenErr = err // still fall through to the wait - returning here closes the pools under loops running on a live ctx
		stop()          // nothing else will cancel ctx on this path
	}
	wg.Wait()
	slog.Info("signals stopped")
	return listenErr
}
