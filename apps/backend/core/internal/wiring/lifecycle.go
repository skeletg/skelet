package wiring

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	netpprof "net/http/pprof"
	"os"
	"sync"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
)

// Shutdown order is load-bearing: drain in-flight HTTP, wait for background workers, then drain async handlers - the caller's connection closes run after this returns
// stop cancels the signal ctx the loops run on - the signal path arrives with it already cancelled, but a listener that never bound does not, and the drain would wait its full timeout on loops nothing told to exit
func Serve(ctx context.Context, stop context.CancelFunc, port string, handler http.Handler, bg *sync.WaitGroup, bus *events.Bus) error {
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Minute, // generous - exports and media uploads stream for minutes; the point is a ceiling at all, so a stalled peer can't hold a goroutine forever
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx }, // tied to the signal ctx so an in-flight request sees shutdown via r.Context().Done()
	}

	slog.Info("admin GraphQL endpoint", "url", fmt.Sprintf("http://localhost:%s%s", port, adminAPIPath()))
	if EnvBool("MCP_ENABLED", true) {
		slog.Info("admin MCP endpoint", "url", fmt.Sprintf("http://localhost:%s%s", port, mcpPath(adminAPIPath())))
	}
	slog.Info("storefront GraphQL endpoint", "url", fmt.Sprintf("http://localhost:%s%s", port, frontAPIPath()))

	serveErr := make(chan error, 1) // goroutine so we can also block on the signal ctx - whichever fires first starts the shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	var listenErr error // a listener that never bound still drains - the loops are already running, and the caller's pool closes the moment this returns
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received; draining HTTP server")
	case err := <-serveErr:
		if err != nil {
			listenErr = fmt.Errorf("http listen: %w", err)
		}
	}
	stop() // no-op after a signal - on a failed bind it's what tells the loops to exit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Background, not the signal ctx - that one is already done, and the Close defers run LIFO after this
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("http server shutdown", "err", err)
	}

	drainBackground(bg, bus)
	return listenErr
}

// Mounts /debug/pprof (go1.27 serves goroutineleak there) when PPROF_ENABLED is set - off by
// default since these ports are published in dev, and wired by hand so no import side effect
func RegisterPprof(mux *http.ServeMux) bool {
	if os.Getenv("PPROF_ENABLED") != "true" {
		return false
	}
	mux.HandleFunc("/debug/pprof/", netpprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", netpprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", netpprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", netpprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", netpprof.Trace)
	return true
}

// The worker tail: /healthz so orchestrators can restart a wedged one, then block on the signal ctx
func ServeWorker(ctx context.Context, stop context.CancelFunc, port string, bg *sync.WaitGroup, bus *events.Bus) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	if RegisterPprof(mux) {
		slog.Info("worker pprof enabled", "url", fmt.Sprintf("http://localhost:%s/debug/pprof/", port))
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	slog.Info("worker health endpoint", "url", fmt.Sprintf("http://localhost:%s/healthz", port))

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	var listenErr error
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received; draining worker")
	case err := <-serveErr:
		if err != nil {
			listenErr = fmt.Errorf("worker health listen: %w", err) // drained below like the server's, and for the same reason
		}
	}
	stop() // same as Serve - a failed bind leaves the signal ctx live, and the loops key on it

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("worker health server shutdown", "err", err)
	}

	drainBackground(bg, bus)
	return listenErr
}

// Shared by both roles so the guarantee holds identically: nothing mid-write when the pool closes run
func drainBackground(bg *sync.WaitGroup, bus *events.Bus) {
	bgDone := make(chan struct{}) // they exit on the signal ctx themselves - waiting is what keeps them off a closed pool
	go func() {
		bg.Wait()
		close(bgDone)
	}()
	select {
	case <-bgDone:
	case <-time.After(10 * time.Second):
		slog.Warn("background workers did not stop in time")
	}

	drainCtx, drainCancel := context.WithTimeout(context.Background(), 15*time.Second) // detached from request contexts, so a short grace then cancel - webhook retry backoffs would outlive the connections
	defer drainCancel()
	if err := bus.Drain(drainCtx); err != nil {
		slog.Warn("event bus drain", "err", err)
	}

	pluginCtx, pluginCancel := context.WithTimeout(context.Background(), 10*time.Second) // last: after the bus so a hook can still publish, before the pool closes so it can still write - its OWN budget, not drainCtx leftovers, or a full-window drain would hand every hook a dead context
	defer pluginCancel()
	pluginhost.Shutdown(pluginCtx)

	slog.Info("graceful shutdown complete")
}
