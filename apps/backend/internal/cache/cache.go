// Package cache is a thin Valkey-backed JSON cache for read paths that can skip Postgres on rarely-changing values - fail open, so a Valkey outage must not break the storefront and every helper returns a miss or no-op on a Redis error and logs it
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct { // one per service or concern - the prefix segregates keys so Del on one cache can't touch another's data
	client *redis.Client
	prefix string
}

func New(client *redis.Client, prefix string) *Cache { // no pool of its own, so pass the client used elsewhere - a nil client is allowed and makes every method a no-op, handy for tests and CLI tooling
	return &Cache{client: client, prefix: prefix}
}

func (c *Cache) key(suffix string) string { // namespaced <ns>:<rest> so a Del can't wipe unrelated keys, and operators can scan one namespace without enumerating the whole instance
	return "cache:" + c.prefix + ":" + suffix
}

func GetJSON[T any](ctx context.Context, c *Cache, suffix string) (T, bool) { // false on any miss, error or decode failure - outages are logged and absorbed, so callers fall through to the DB
	var zero T
	if c == nil || c.client == nil {
		return zero, false
	}
	raw, err := c.client.Get(ctx, c.key(suffix)).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) && !errors.Is(err, context.Canceled) {
			slog.Warn("cache get error", "ns", c.prefix, "suffix", suffix, "err", err)
		}
		return zero, false
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		// Schema drift between writer and reader - drop the stale entry and the next miss repopulates
		_ = c.client.Del(ctx, c.key(suffix)).Err()
		return zero, false
	}
	return out, true
}

func (c *Cache) SetJSON(ctx context.Context, suffix string, v any, ttl time.Duration) { // errors are logged rather than returned - a miss next time is harmless, and callers shouldn't handle two failure modes for one read path
	if c == nil || c.client == nil {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		slog.Warn("cache marshal error", "ns", c.prefix, "suffix", suffix, "err", err)
		return
	}
	if err := c.client.Set(ctx, c.key(suffix), raw, ttl).Err(); err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("cache set error", "ns", c.prefix, "suffix", suffix, "err", err)
		}
	}
}

// For write paths after a mutation, so the next read goes back to Postgres - missing keys are silently ignored, per Valkey's DEL semantics
func (c *Cache) Del(ctx context.Context, suffixes ...string) {
	if c == nil || c.client == nil || len(suffixes) == 0 {
		return
	}
	keys := make([]string, len(suffixes))
	for i, s := range suffixes {
		keys[i] = c.key(s)
	}
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("cache del error", "ns", c.prefix, "keys", keys, "err", err)
		}
	}
}

// Use sparingly - SCAN+DEL is O(keys-in-namespace) and runs in the foreground - Reserved for invalidations that touch many related entries, where enumerating exact keys is impractical
func (c *Cache) DelPattern(ctx context.Context, pattern string) {
	if c == nil || c.client == nil {
		return
	}
	full := c.key(pattern)
	var cursor uint64
	for {
		batch, next, err := c.client.Scan(ctx, cursor, full, 200).Result()
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Warn("cache scan error", "ns", c.prefix, "pattern", pattern, "err", err)
			}
			return
		}
		if len(batch) > 0 {
			_ = c.client.Del(ctx, batch...).Err()
		}
		if next == 0 {
			return
		}
		cursor = next
	}
}
