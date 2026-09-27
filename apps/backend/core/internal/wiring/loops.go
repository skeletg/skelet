package wiring

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/datajob"
	"github.com/skeletg/skelet/apps/backend/core/internal/notification"
	"github.com/skeletg/skelet/apps/backend/core/internal/outbox"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/scheduler"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient/roster"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// SuperviseLoop keeps one background loop alive across panics - a bare loop inherits no recover at all, so one panicking export resource or webhook handler would kill the whole process - a panic logs with the stack and the loop restarts after a beat; ctx cancellation is the clean exit
func SuperviseLoop(ctx context.Context, run func(context.Context)) {
	for {
		panicked := func() (panicked bool) {
			defer func() {
				if rec := recover(); rec != nil {
					panicked = true
					slog.Error("background loop panicked - restarting",
						"panic", fmt.Sprint(rec),
						"stack", string(debug.Stack()),
					)
				}
			}()
			run(ctx)
			return false
		}()
		if !panicked || ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

type BackgroundWaitGroup struct{ *sync.WaitGroup } // supplied by the entrypoint so the drain waits every loop out before the pools close - Add happens before the spawn, which is the ordering a drain beginning first would otherwise miss

var ServerLoops = fx.Module("serverloops", fx.Invoke(startServerLoops)) // only a process serving HTTP may run these - including the module IS the role decision, so there is no role check inside

var BackgroundLoops = fx.Module("backgroundloops", fx.Invoke(startBackgroundLoops)) // scheduler claim loop, webhook delivery, outbox relay - the worker always runs these, the server unless JOBS_EXTERNAL hands them over

// OnStart because fx runs invokes inside fx.New, before the entrypoint has registered the subscribers these loops dispatch into
// OnStop because these hooks land after the pool hooks, so fx's LIFO unwind keeps a loop off a closed pool even when the entrypoint's drain never runs
func parkLoops(lc fx.Lifecycle, rc RootContext, bg BackgroundWaitGroup, what string, loops ...func(context.Context)) {
	ctx, cancel := context.WithCancel(rc.Context) // Its own cancel, so an app.Stop while the signal ctx is still live stops them anyway
	var own sync.WaitGroup
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			for _, loop := range loops {
				run := loop
				own.Add(1) // Both Adds before the spawn
				bg.Add(1)
				go func() {
					defer own.Done()
					defer bg.Done()
					SuperviseLoop(ctx, run) // the signal ctx's child, never the hook's - that one dies the moment Start returns
				}()
			}
			slog.Info("background loops started in this process", "loops", what)
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			cancel()
			done := make(chan struct{})
			go func() { own.Wait(); close(done) }()
			select {
			case <-done:
			case <-stopCtx.Done():
				slog.Warn("background loops did not stop before the shutdown deadline", "loops", what)
			}
			return nil
		},
	})
}

func startServerLoops(
	lc fx.Lifecycle, rc RootContext, bg BackgroundWaitGroup,
	hub *notification.Hub, promos *promotion.Service,
) {
	parkLoops(lc, rc, bg, "notification-hub+promotion-cache",
		hub.Run, // fans out to WS subscribers, which the worker has none of - publishing still works there
		func(ctx context.Context) { promos.Engine().RunCacheInvalidation(ctx) }, // relays invalidations from other replicas so an edit lands fleet-wide
	)
}

func startBackgroundLoops(
	lc fx.Lifecycle, rc RootContext, bg BackgroundWaitGroup,
	jobs *scheduler.Service, hooks *webhook.Service, dispatcher *webhook.Dispatcher, relay *outbox.Outbox,
	transfers *datajob.Service, chans *channel.Service, cfg *config.Service, signals *signalsclient.Client, enabled SignalsEnabled,
) {
	// The scheduled push is stamped a whole interval out by Seed, so on the run that first creates the
	// roster table the suite would answer every beacon with "not a channel I serve" until it fires
	if bool(enabled) {
		lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
			if err := roster.PushChannels(ctx, chans, cfg, signals); err != nil {
				slog.Warn("channel roster push at boot failed - the scheduled push retries", "err", err)
			}
			return nil
		}})
	}
	parkLoops(lc, rc, bg, "scheduler+webhook-queue+outbox+data-transfer",
		jobs.Run, // only claiming moves - the admin jobs UI still works either way, since List/Get read the DB
		func(ctx context.Context) { webhook.NewQueueWorker(hooks, dispatcher).Run(ctx) }, // enqueueing stays in-process, only delivery moves
		transfers.Run, // import/export runs claim with SKIP LOCKED, so every replica running this is the point
		relay.Run,     // last, so a replayed event can't fire before its subscribers exist
	)
}
