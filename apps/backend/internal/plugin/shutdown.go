package plugin

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Without this, init() is the whole plugin lifecycle and anything a plugin STARTS - a pool, a queue consumer, the goroutines the manifest's "long-running" capability describes - leaks through every restart
// Concurrent and bounded like health probes, so one wedged plugin delays the exit rather than holding it

const shutdownTimeout = 5 * time.Second

type shutdownHook struct {
	slug string
	fn   sdk.ShutdownFunc
}

type shutdownRegistry struct {
	mu    sync.Mutex
	hooks []shutdownHook
}

var shutdownRegistryInstance = &shutdownRegistry{}

func (h *Host) RegisterShutdown(slug string, fn sdk.ShutdownFunc) error {
	shutdownRegistryInstance.mu.Lock()
	defer shutdownRegistryInstance.mu.Unlock()
	shutdownRegistryInstance.hooks = append(shutdownRegistryInstance.hooks, shutdownHook{slug: slug, fn: fn})
	return nil
}

// Errors and panics are logged rather than propagated - by here there's no caller left to fail
func Shutdown(ctx context.Context) {
	shutdownRegistryInstance.mu.Lock()
	hooks := append([]shutdownHook(nil), shutdownRegistryInstance.hooks...)
	shutdownRegistryInstance.mu.Unlock()
	if len(hooks) == 0 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(hooks))
	for _, hk := range hooks {
		go func() {
			defer wg.Done()
			hookCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
			defer cancel()
			if err := safeShutdown(hookCtx, hk.fn); err != nil {
				slog.Warn("plugin shutdown hook", "plugin", hk.slug, "err", err)
			}
		}()
	}
	wg.Wait()
}

func safeShutdown(ctx context.Context, fn sdk.ShutdownFunc) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = errFromPanic(rec)
		}
	}()
	return fn(ctx)
}
