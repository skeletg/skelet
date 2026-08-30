package sdk

import "context"

// Stops whatever the plugin started that outlives a request - a pool, a queue consumer, the goroutines Manifest's "long-running" capability describes
// The ctx carries a short deadline and a hook blocking past it is abandoned, so the process still exits; a returned error is logged, since by then nothing is left to fail
type ShutdownFunc func(ctx context.Context) error

// Runs after the HTTP server stops accepting and before the pools close, so a hook can still finish a write while nothing new arrives - concurrent across plugins, so don't rely on ordering
func (pl *Plugin) OnShutdown(fn ShutdownFunc) {
	withHost(func(h Host) error { return h.RegisterShutdown(pl.slug, fn) })
}
