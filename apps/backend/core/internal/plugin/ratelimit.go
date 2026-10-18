package plugin

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Per-plugin rate limiting from plugin/<slug>/rate_limit_per_min, auto-registered alongside the enabled flag and timeout - 0 means unlimited, the common case Instrument spends one slot per hook, so this bounds every surface routed through it with no plugin cooperation - async subscribers and scheduled jobs bypass Instrument and stay unbounded, being off the request path already The window is in-process, so N replicas admit N x the configured rate - it exists to stop ONE plugin monopolising ONE instance's request lanes, not to meter a third party's API quota, which needs ratelimit.NewRedis and a key the whole fleet shares

var ErrPluginRateLimited = errors.New("plugin: rate limited")

const rateLimitWindow = time.Minute

var (
	rateBucketsMu sync.Mutex
	rateBuckets   = map[string]*rateBucket{}
)

// A sliding-window counter - this minute's count plus last minute's weighted by how much of it the window still overlaps
// Two ints per plugin however high the limit, where a log of call times grew with it
type rateBucket struct {
	windowStart time.Time
	prev, cur   int
}

func (b *rateBucket) admit(now time.Time, limit int) bool {
	if elapsed := now.Sub(b.windowStart); elapsed >= rateLimitWindow {
		b.prev = 0
		if elapsed < 2*rateLimitWindow {
			b.prev = b.cur
		}
		b.cur = 0
		b.windowStart = now.Truncate(rateLimitWindow)
	}
	overlap := 1 - float64(now.Sub(b.windowStart))/float64(rateLimitWindow)
	if float64(b.prev)*overlap+float64(b.cur) >= float64(limit) {
		return false
	}
	b.cur++
	return true
}

// Fail-open - a config-read error or an unset limit returns nil without consuming a slot
// Called by Instrument at hook entry; a rejected call doesn't consume a slot either, so a plugin stuck at its ceiling recovers as soon as the window slides
func CheckRateLimit(ctx context.Context, slug string) error {
	limit := rateLimitFor(ctx, slug)
	if limit <= 0 {
		return nil
	}
	rateBucketsMu.Lock()
	defer rateBucketsMu.Unlock()
	b, ok := rateBuckets[slug]
	if !ok {
		b = &rateBucket{}
		rateBuckets[slug] = b
	}
	if !b.admit(time.Now(), limit) {
		return ErrPluginRateLimited
	}
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
