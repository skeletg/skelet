// Package cache is a thin Valkey-backed JSON cache for read paths that can skip Postgres on rarely-changing values - fail open, so a Valkey outage must not break the storefront and every helper returns a miss or no-op on a Redis error and logs it
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strconv"
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

// Every key carries its namespace's generation, so Flush drops the whole namespace with one write instead of a SCAN
func (c *Cache) genKey(group string) string {
	if group == "" {
		return "cache:gen:" + c.prefix
	}
	return "cache:gen:" + c.prefix + ":" + group
}

func (c *Cache) genKeys(group string) []string {
	if group == "" {
		return []string{c.genKey("")}
	}
	return []string{c.genKey(""), c.genKey(group)}
}

func (c *Cache) base() string { return "cache:" + c.prefix + ":v" }

func member(group, suffix string) string {
	if group == "" {
		return suffix
	}
	return group + ":" + suffix
}

// Only a write mints a missing generation, fresh so an evicted counter never resurrects old entries, and stretches a group's generation to outlive the entry
const resolveLua = `
local function resolve(mint)
  local g = ''
  for i = 1, #KEYS do
    local v = redis.call('GET', KEYS[i])
    if not v then
      if not mint then return false end
      v = ARGV[3]
      if i > 1 then redis.call('SET', KEYS[i], v, 'PX', ARGV[5]) else redis.call('SET', KEYS[i], v) end
    elseif mint and i > 1 then
      redis.call('PEXPIRE', KEYS[i], ARGV[5], 'GT')
    end
    if i > 1 then g = g .. '.' end
    g = g .. v
  end
  return ARGV[1] .. g .. ':' .. ARGV[2]
end
`

var (
	getScript = redis.NewScript(resolveLua + `local k = resolve(false) if not k then return false end return redis.call('GET', k)`)
	setScript = redis.NewScript(resolveLua + `return redis.call('SET', resolve(true), ARGV[4], 'PX', ARGV[5])`)
	delScript = redis.NewScript(resolveLua + `local k = resolve(false) if not k then return 0 end return redis.call('DEL', k)`)
	keyScript = redis.NewScript(resolveLua + `return resolve(false)`)
)

const maxEntryTTL = 24 * time.Hour

func nonce() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36) + strconv.FormatInt(rand.Int64(), 36)
}

func (c *Cache) args(group, suffix string, extra ...any) []any {
	return append([]any{c.base(), member(group, suffix), nonce()}, extra...)
}

func GetJSON[T any](ctx context.Context, c *Cache, suffix string) (T, bool) { // false on any miss, error or decode failure - outages are logged and absorbed, so callers fall through to the DB
	return GetJSONIn[T](ctx, c, "", suffix)
}

func GetJSONIn[T any](ctx context.Context, c *Cache, group, suffix string) (T, bool) {
	var zero T
	if c == nil || c.client == nil {
		return zero, false
	}
	raw, err := getScript.Run(ctx, c.client, c.genKeys(group), c.args(group, suffix)...).Text()
	if err != nil {
		if !errors.Is(err, redis.Nil) && !errors.Is(err, context.Canceled) {
			slog.Warn("cache get error", "ns", c.prefix, "suffix", suffix, "err", err)
		}
		return zero, false
	}
	var out T
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		// Schema drift between writer and reader - drop the stale entry and the next miss repopulates
		c.DelIn(ctx, group, suffix)
		return zero, false
	}
	return out, true
}

func (c *Cache) SetJSON(ctx context.Context, suffix string, v any, ttl time.Duration) { // errors are logged rather than returned - a miss next time is harmless, and callers shouldn't handle two failure modes for one read path
	c.SetJSONIn(ctx, "", suffix, v, ttl)
}

func (c *Cache) SetJSONIn(ctx context.Context, group, suffix string, v any, ttl time.Duration) {
	if c == nil || c.client == nil {
		return
	}
	if ttl <= 0 || ttl > maxEntryTTL {
		ttl = maxEntryTTL
	}
	raw, err := json.Marshal(v)
	if err != nil {
		slog.Warn("cache marshal error", "ns", c.prefix, "suffix", suffix, "err", err)
		return
	}
	if err := setScript.Run(ctx, c.client, c.genKeys(group), c.args(group, suffix, raw, ttl.Milliseconds())...).Err(); err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("cache set error", "ns", c.prefix, "suffix", suffix, "err", err)
		}
	}
}

// For write paths after a mutation, so the next read goes back to Postgres - missing keys are silently ignored, per Valkey's DEL semantics
func (c *Cache) Del(ctx context.Context, suffixes ...string) {
	for _, s := range suffixes {
		c.DelIn(ctx, "", s)
	}
}

func (c *Cache) DelIn(ctx context.Context, group, suffix string) {
	if c == nil || c.client == nil {
		return
	}
	if err := delScript.Run(ctx, c.client, c.genKeys(group), c.args(group, suffix)...).Err(); err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("cache del error", "ns", c.prefix, "suffix", suffix, "err", err)
		}
	}
}

func (c *Cache) Flush(ctx context.Context) {
	c.FlushGroup(ctx, "")
}

func (c *Cache) FlushGroup(ctx context.Context, group string) {
	if c == nil || c.client == nil {
		return
	}
	if err := c.client.Del(ctx, c.genKey(group)).Err(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("cache flush error", "ns", c.prefix, "err", err)
	}
}

func (c *Cache) resolvedKey(ctx context.Context, group, suffix string) string {
	k, _ := keyScript.Run(ctx, c.client, c.genKeys(group), c.args(group, suffix)...).Text()
	return k
}

// DelPattern SCANs the whole keyspace, so it is for dev tooling only - write paths use Flush or FlushGroup
func (c *Cache) DelPattern(ctx context.Context, pattern string) {
	if c == nil || c.client == nil {
		return
	}
	full := "cache:" + c.prefix + ":" + pattern
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
