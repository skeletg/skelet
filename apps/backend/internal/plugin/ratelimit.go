package plugin

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/skeletg/skelet/internal/config"
)

// Per-plugin rate limiting from plugin/<slug>/rate_limit_per_min, auto-registered alongside the enabled flag and timeout - 0 means unlimited, the common case
// Instrument spends one slot per hook, so this bounds every surface routed through it with no plugin cooperation - async subscribers and scheduled jobs bypass Instrument and stay unbounded, being off the request path already
// The window is in-process, so N replicas admit N x the configured rate - it exists to stop ONE plugin monopolising ONE instance's request lanes, not to meter a third party's API quota, which needs ratelimit.NewRedis and a key the whole fleet shares

var ErrPluginRateLimited = errors.New("plugin: rate limited")

const rateLimitWindow = time.Minute

var (
	rateBucketsMu sync.Mutex
	rateBuckets   = map[string]*rateBucket{}
)

type rateBucket struct {
	calls []time.Time
}

// Fail-open - a config-read error or an unset limit returns nil without consuming a slot
// Called by Instrument at hook entry; a rejected call doesn't consume a slot either, so a plugin stuck at its ceiling recovers as soon as the window slides
func CheckRateLimit(ctx context.Context, slug string) error {
	limit := rateLimitFor(ctx, slug)
	if limit <= 0 {
		return nil
	}
	now := time.Now()
	cutoff := now.Add(-rateLimitWindow)

	rateBucketsMu.Lock()
	defer rateBucketsMu.Unlock()
	b, ok := rateBuckets[slug]
	if !ok {
		b = &rateBucket{}
		rateBuckets[slug] = b
	}
	keep := 0
	for ; keep < len(b.calls); keep++ {
		if b.calls[keep].After(cutoff) {
			break
		}
	}
	if keep > 0 {
		b.calls = b.calls[keep:]
	}
	if len(b.calls) >= limit {
		return ErrPluginRateLimited
	}
	b.calls = append(b.calls, now)
	return nil
}

// Falls back to 0 (unlimited) on any error, so config issues never cause spurious rate-limit errors
func rateLimitFor(ctx context.Context, slug string) int {
	h := installedHost.Load()
	if h == nil {
		return 0
	}
	svc := h.configSvc.Load()
	if svc == nil {
		return 0
	}
	n, err := svc.GetInt(ctx, "plugin/"+slug+"/rate_limit_per_min", config.DefaultScope)
	if err != nil || n < 0 {
		return 0
	}
	return int(n)
}
