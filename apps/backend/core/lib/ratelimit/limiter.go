// Package ratelimit caps how often a key can call a sensitive operation - Valkey-backed by default so every pod sees the same counters, and credential stuffing can't be doubled by hitting two pods
// A nil client (VALKEY_URL unset) falls back to the in-process limiter, which is weaker since the budget becomes rate x replicas - Key shape is the caller's - the storefront uses "<op>:<ip>"
package ratelimit

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter interface {
	Allow(ctx context.Context, key string) bool
}

// Caps `rate` Allow=true returns per `window` per key - INCR and PEXPIRE run inside a Lua script, since without it a crash between the two leaves the key persistent forever and the counter never rolls over
type RedisLimiter struct {
	client *redis.Client
	rate   int
	window time.Duration
	script *redis.Script
	// Enforces per-process on a Valkey error, making the budget rate x pods, rather than failing open - empty and free while Valkey is healthy
	fallback *MemoryLimiter
}

// A fixed window - the first Allow sets the counter to 1 with a window-long TTL and the rest just INCR, so expiry starts the next window - One tiny key per (op, IP) per window, bounded by that expiry
func NewRedis(client *redis.Client, rate int, window time.Duration) *RedisLimiter {
	const lua = `
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`
	fb := NewMemory(rate, window)
	// Sweep for the process lifetime so the fallback map can't grow unbounded - a nil stop never fires, and one goroutine per limiter is negligible
	go fb.Sweep(nil)
	return &RedisLimiter{
		client:   client,
		rate:     rate,
		window:   window,
		script:   redis.NewScript(lua),
		fallback: fb,
	}
}

func (l *RedisLimiter) Allow(ctx context.Context, key string) bool {
	return l.AllowN(ctx, key, l.rate)
}

// Allow with a per-call rate override for runtime-configurable budgets - rate <= 0 disables the check, so a config knob's zero value means off This is also how the whole layer ships disabled: wiring builds every limiter with rate 0 unless RATE_LIMIT_ENABLED, and 0 returns before any Valkey round-trip Take is deliberately NOT gated by rate, so the throttle knobs that read their own budget from config keep working while per-IP limiting is off
func (l *RedisLimiter) AllowN(ctx context.Context, key string, rate int) bool {
	if rate <= 0 {
		return true
	}
	n, ok := l.Take(ctx, key)
	if !ok {
		return true
	}
	return n <= int64(rate)
}

// Increments the key's window counter and returns the new count - unlike Allow it makes no over/under decision, because callers that THROTTLE (slow down before refusing) need the raw count to scale their response On a Valkey error, or with no Valkey configured, it delegates to the in-process fallback and returns that count with ok=true, so callers keep enforcing per-process either way A nil client is a supported deployment, NOT a reason to stop limiting - failing open there would silently ship an unthrottled login endpoint. ok=false only with no limiter and no fallback, or a cancelled context
func (l *RedisLimiter) Take(ctx context.Context, key string) (int64, bool) {
	if l == nil {
		return 0, false
	}
	if l.client == nil {
		if l.fallback != nil {
			return l.fallback.Take(ctx, key)
		}
		return 0, false
	}
	// PEXPIRE wants ms - cast through int64 to keep the script signature numeric, since Redis Lua otherwise treats ARGV entries as strings and the int comparison silently breaks
	ms := l.window.Milliseconds()
	n, err := l.script.Run(ctx, l.client, []string{"ratelimit:" + key}, ms).Int64()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return 0, false
		}
		slog.Warn("ratelimit valkey error; enforcing via in-process fallback", "err", err, "key", key)
		if l.fallback != nil {
			return l.fallback.Take(ctx, key)
		}
		return 0, false
	}
	return n, true
}

// The same fixed-window scheme as RedisLimiter but in a process-local map - for tests and CLI tooling with no Valkey wired - Production should always pick NewRedis so every pod sees the same counters
type MemoryLimiter struct {
	mu      sync.Mutex
	buckets map[string]*memBucket
	rate    int
	window  time.Duration
}

type memBucket struct {
	count   int
	resetAt time.Time
}

// Memory grows with the number of unique keys seen within a window - Sweep() trims expired entries and is meant to run in a goroutine over the limiter's lifetime
func NewMemory(rate int, window time.Duration) *MemoryLimiter {
	return &MemoryLimiter{
		buckets: map[string]*memBucket{},
		rate:    rate,
		window:  window,
	}
}

func (l *MemoryLimiter) Allow(ctx context.Context, key string) bool {
	return l.AllowN(ctx, key, l.rate)
}

func (l *MemoryLimiter) AllowN(ctx context.Context, key string, rate int) bool {
	if rate <= 0 {
		return true
	}
	n, _ := l.Take(ctx, key)
	return n <= int64(rate)
}

func (l *MemoryLimiter) Take(_ context.Context, key string) (int64, bool) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok || now.After(b.resetAt) {
		l.buckets[key] = &memBucket{count: 1, resetAt: now.Add(l.window)}
		return 1, true
	}
	b.count++
	return int64(b.count), true
}

// Drops expired buckets so memory stays bounded under churn - cancel by closing the stop channel
func (l *MemoryLimiter) Sweep(stop <-chan struct{}) {
	ticker := time.NewTicker(l.window * 2)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			l.mu.Lock()
			for k, b := range l.buckets {
				if now.After(b.resetAt) {
					delete(l.buckets, k)
				}
			}
			l.mu.Unlock()
		}
	}
}
