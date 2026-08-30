package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/skeletg/skelet/internal/applog"
	"github.com/skeletg/skelet/internal/envcheck"
	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/observability"
	"github.com/skeletg/skelet/internal/wiring"

	_ "github.com/skeletg/skelet/plugins/firstparty"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

// Boot order: env → logger → observability → graph → cross-domain wiring → loops → HTTP server → SIGTERM → drain in LIFO - failures return through here rather than os.Exit so the deferred closes fire, except a graph that fails to CONSTRUCT, where fx has no started hooks to unwind and the open pools are left to process exit
func run() error {
	_ = godotenv.Load()

	applog.Setup(os.Getenv("APP_ENV")) // Initialise the root logger before any other code can emit, so every boot-path failure below logs structurally with file:line

	if err := envcheck.Report(envcheck.Production(os.Getenv, // everything, because this process is the API, the admin surface and the mail sender at once
		envcheck.JWTSecret, envcheck.TrustedProxies, envcheck.ConfigEncryptionKey, envcheck.Database,
		envcheck.CORS, envcheck.FrontendURLs, envcheck.SMTP, envcheck.S3, envcheck.OpenSearch,
		envcheck.RateLimiting, envcheck.LoginLockout,
	)); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM) // Cancelled by SIGINT/SIGTERM, which is what wakes ListenAndServe into the drain below
	defer stop()

	telemetry := observability.Init(ctx, os.Getenv("APP_ENV")) // Has to precede the graph - otelx.PGXTracer only returns a live tracer once this has run, so the pools would open untraced
	defer func() {
		sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer scancel()
		if err := telemetry.Shutdown(sctx); err != nil {
			slog.Warn("observability shutdown", "err", err)
		}
	}()

	cfg, err := wiring.LoadConfig() // read before the graph so the module list below can BE the role decision, not something re-derived from scattered ifs
	if err != nil {
		return err
	}
	var bg sync.WaitGroup

	opts := []fx.Option{ // the roles differ only in which modules they compose - the worker takes no HTTP surface
		wiring.Pools,
		wiring.Cache,
		wiring.Registries,
		wiring.Platform,
		wiring.Integrations,
		wiring.Services,
		wiring.Domain,
		wiring.DataTransfer,
		wiring.Jobs,
		wiring.Search,
		wiring.Health,
		wiring.Wiring,
	}
	if cfg.RunBackground {
		opts = append(opts, wiring.BackgroundLoops)
	} else {
		slog.Info("background layer deferred to the worker role (JOBS_EXTERNAL=true)")
	}
	if cfg.IsWorker() {
		slog.Info("booting in worker role", "api", "disabled", "background", "scheduler+webhook-queue+outbox+data-transfer")
	} else {
		opts = append(opts, wiring.GraphQL, wiring.HTTP, wiring.ServerLoops)
	}

	var bus *events.Bus
	var root wiring.RootHandler
	populate := []any{&bus} // the worker has no HTTP modules, so asking its graph for the handler is a missing-type failure
	if !cfg.IsWorker() {
		populate = append(populate, &root)
	}
	opts = append(opts,
		fx.Supply(cfg, wiring.RootContext{Context: ctx}, telemetry, wiring.QueueProducer,
			wiring.BackgroundWaitGroup{WaitGroup: &bg}),
		fx.Populate(populate...),
		fx.WithLogger(func() fxevent.Logger {
			l := &fxevent.SlogLogger{Logger: slog.Default()}
			l.UseLogLevel(slog.LevelDebug)
			return l
		}),
	)

	app := fx.New(opts...) // construction AND cross-domain wiring - every subscriber is registered when this returns; nothing serves or loops until app.Start
	if err := app.Err(); err != nil {
		return err
	}
	defer func() {
		sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer scancel()
		if err := app.Stop(sctx); err != nil {
			slog.Warn("infrastructure shutdown", "err", err)
		}
	}()

	// Start launches the background loops and is deliberately after fx.New - the outbox relay marks a row delivered whether or not anyone listened, so a loop running before the invokes finish silently drops stock compensation, gift cards and capture-on-first-ship
	if err := app.Start(ctx); err != nil {
		return err
	}

	if cfg.IsWorker() { // Worker role stops here: no listener, no routes, just /healthz and the background loops until SIGTERM
		return wiring.ServeWorker(ctx, stop, cfg.WorkerPort, &bg, bus)
	}

	return wiring.Serve(ctx, stop, cfg.Port, root.Handler, &bg, bus) // serves until the signal ctx cancels, then drains - the deferred app.Stop and telemetry closes fire after this returns (LIFO)
}
