// Command signals runs the Signals subsystem as its own OS process, so an analytics overload or panic can't take checkout down
// It owns the commerce-decoupled slice: retention pruning of the spine + Lens (the heavy DELETEs), forwarding spine events to external destinations, trait compute, and the ingest-queue consumer when SIGNALS_QUEUE_URL is set - required then, since nothing else drains the stream
// Prune and trait compute are idempotent and the forwarding cursor advance is guarded, but co-running still double-posts each batch once - set SIGNALS_JOBS_EXTERNAL in cmd/server whenever this worker is deployed
// The bus-to-spine transform deliberately stays with the outbox relay in cmd/server: it's cheap, and the flow-enrolling jobs need the app's subscriber graph - The dashboard read API is the one unstarted chunk
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

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/applog"
	"github.com/skeletg/skelet/apps/backend/internal/cache"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/destination"
	"github.com/skeletg/skelet/apps/backend/internal/envcheck"
	"github.com/skeletg/skelet/apps/backend/internal/httpx"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
	"github.com/skeletg/skelet/apps/backend/internal/observability"
	"github.com/skeletg/skelet/apps/backend/internal/signalsapi"
	"github.com/skeletg/skelet/apps/backend/internal/signalsboot"
	"github.com/skeletg/skelet/apps/backend/internal/signalsgrpc"
	"github.com/skeletg/skelet/apps/backend/internal/wiring"
)

func main() {
	if err := run(); err != nil {
		slog.Error("signals worker exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	applog.Setup(os.Getenv("APP_ENV"))

	// No public surface beyond /healthz, but it decrypts secret config and opens two pools - validated before anything connects
	if err := envcheck.Report(envcheck.Production(os.Getenv,
		envcheck.ConfigEncryptionKey, envcheck.Database,
	)); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM) // SIGTERM/SIGINT cancels ctx - drives graceful shutdown of the loops and the health server so in-flight prunes/forwards finish before we close pools
	defer stop()

	telemetry := observability.Init(ctx, os.Getenv("APP_ENV")) // Traces/metrics when OTEL_* is set, no-op otherwise - as a separate process this is how you follow an event across the service boundary
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(sctx); err != nil {
			slog.Warn("observability shutdown", "err", err)
		}
	}()

	// Same modules cmd/server builds from, minus the ones a worker must not run (no Registries, Cache or Platform) - wiring QueueConsumer is the one real difference: this process drains the stream, so its own spine handle must stay the bare engine rather than wrapping into it
	var d wiring.SignalsDeps
	app := fx.New(
		fx.Provide(wiring.LoadSignalsConfig),
		wiring.Pools,
		wiring.Settings,
		wiring.SignalsCore,
		wiring.SignalsWiring,
		fx.Supply(wiring.RootContext{Context: ctx}, wiring.QueueConsumer),
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

	primaryConn, sigConn := d.DB, d.SignalsPool.DB
	configSvc, channelSvc := d.ConfigSvc, d.ChannelSvc
	stack := d.Stack
	rec, chPing := stack.Recorder, stack.CHPing
	lensSvc := d.Lens
	destSvc := d.Destination

	var wg sync.WaitGroup
	wg.Go(func() {
		wiring.SuperviseLoop(ctx, func(ctx context.Context) { runRetention(ctx, configSvc, rec, lensSvc) })
	})
	if stack.Enabled { // Retention keeps running with the suite off (hygiene), forwarding doesn't - a disabled deployment mustn't keep posting events to GA4/Meta/webhooks
		wg.Go(func() { wiring.SuperviseLoop(ctx, func(ctx context.Context) { runForwarding(ctx, destSvc) }) })
		wg.Go(func() {
			wiring.SuperviseLoop(ctx, func(ctx context.Context) {
				runTraitCompute(ctx, signalsboot.TraitComputeJob(rec, channelSvc, configSvc))
			})
		})
	} else {
		slog.Info("destination forwarding + trait compute off (SIGNALS_ENABLED=false)")
	}
	if stack.Queue != nil && stack.Enabled { // The ingest-queue consumer: drains JetStream into the selected engine - This worker never wraps its spine, so rec.SpineStore() is the engine itself
		wg.Go(func() {
			// SuperviseLoop keeps a panicking consumer from killing the worker - a clean Consume error stays terminal
			wiring.SuperviseLoop(ctx, func(ctx context.Context) {
				if err := stack.Queue.Consume(ctx, rec.SpineStore()); err != nil {
					slog.Error("signals queue consumer", "err", err)
				}
			})
		})
		slog.Info("signals ingest queue consumer started")
	}

	mux := http.NewServeMux()
	wiring.RegisterPprof(mux)
	// The suite's own admin API - the dashboard reaches it directly, authenticating with a service token core minted
	mux.Handle(signalsapi.Path, wiring.SignalsGraphQLHandler(wiring.SignalsAPI{
		Experiment:  d.Experiment,
		Lens:        lensSvc,
		Destination: destSvc,
		Segment:     d.Segment,
		Analytics:   rec,
		Channel:     channelSvc,
		Config:      configSvc,
		Commerce:    db.New(primaryConn),
		Roster:      signalsdb.New(sigConn),
		JWTSecret:   d.Config.JWTSecret,
		Dev:         d.Config.IsDev(),
		Telemetry:   telemetry,
		// Core publishes here when it kills a session, so one revoked there stops working here too
		Revocations: cache.NewRevocations(d.Valkey, wiring.AccessTokenTTL+time.Minute),
	}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := primaryConn.PingContext(r.Context()); err != nil { // every datastore this process prunes/forwards against - commerce alone read healthy while the actual work errored forever
			http.Error(w, "commerce db unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := sigConn.PingContext(r.Context()); err != nil {
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
	// The wire contract, on its own listener - grpc needs one, and keeping it separate means the
	// GraphQL surface can be retired without touching what serves it
	grpcAddr := ":" + envDefault("SIGNALS_GRPC_PORT", "8092")
	grpcSrv := signalsgrpc.NewServer(signalsgrpc.Deps{
		Experiment: d.Experiment,
		JWTSecret:  d.Config.JWTSecret,
		Dev:        d.Config.IsDev(),
	})
	grpcLn, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("listen grpc: %w", err)
	}
	wg.Go(func() {
		if err := grpcSrv.Serve(grpcLn); err != nil {
			slog.Error("signals grpc server", "err", err)
		}
	})
	go func() {
		<-ctx.Done()
		grpcSrv.GracefulStop()
	}()
	slog.Info("signals grpc listening", "addr", grpcAddr)

	srv := &http.Server{
		Addr:              ":" + envDefault("SIGNALS_PORT", "8090"),
		Handler:           httpx.Recover(httpx.RequestID(mux)), // panic -> structured 500 with request_id instead of an aborted connection
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

	slog.Info("signals worker started", "addr", srv.Addr, "sig_db_own", d.Config.SignalsURL != d.Config.DatabaseURL)
	var listenErr error
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		listenErr = err // still fall through to the wait - returning here closes the pools under loops running on a live ctx
		stop()          // nothing else will cancel ctx on this path
	}
	wg.Wait()
	slog.Info("signals worker stopped")
	return listenErr
}

// runRetention prunes the spine + Lens once at startup and then daily, matching cmd/server's analytics-prune / lens-prune cadence - Config drives the windows
func runRetention(ctx context.Context, cfg *config.Service, rec *analytics.Recorder, lensSvc *lens.Service) {
	pruneOnce(ctx, cfg, rec, lensSvc)
	tick := time.NewTicker(24 * time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			pruneOnce(ctx, cfg, rec, lensSvc)
		}
	}
}

func pruneOnce(ctx context.Context, cfg *config.Service, rec *analytics.Recorder, lensSvc *lens.Service) {
	if err := signalsboot.SpinePruneJob(cfg, rec)(ctx); err != nil {
		slog.Error("spine retention", "err", err)
	}
	if err := signalsboot.LensPruneJob(cfg, lensSvc)(ctx); err != nil {
		slog.Error("lens retention", "err", err)
	}
}

// runForwarding drains the spine to external destinations every minute, matching cmd/server's event-destinations-forward job - Durable cursors make it resumable
func runForwarding(ctx context.Context, destSvc *destination.Service) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := destSvc.ForwardAll(ctx); err != nil {
				slog.Error("destination forward", "err", err)
			}
		}
	}
}

// runTraitCompute recomputes RFM/lifecycle/engagement traits once at startup and then every 12h, matching cmd/server's trait-compute cadence - co-running with an un-flipped cmd/server just recomputes the same values (idempotent)
func runTraitCompute(ctx context.Context, job func(context.Context) error) {
	if err := job(ctx); err != nil {
		slog.Error("trait compute", "err", err)
	}
	tick := time.NewTicker(12 * time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := job(ctx); err != nil {
				slog.Error("trait compute", "err", err)
			}
		}
	}
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
