package boot

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	netpprof "net/http/pprof"
	"os"
	"runtime/debug"
	"time"
)

// SuperviseLoop keeps one background loop alive across panics - a bare loop inherits no recover, so one panicking flush would kill the process
// A panic logs with its stack and the loop restarts after a beat; ctx cancellation is the clean exit
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

// RegisterPprof mounts the profiler when PPROF_ENABLED=true, the same switch core's processes read
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
