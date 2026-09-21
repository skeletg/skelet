package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Every read path that can skip Postgres goes through here, and the whole design is fail-open: a Valkey outage has to look like a cache miss, never like an error the storefront renders

// Points at a closed port, so every command errors without waiting on a real outage
func deadClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond, MaxRetries: -1})
}

// The live instance, when one is configured - the round-trip behaviour is Valkey's, so there is
// nothing to assert about it without a server
func liveClient(t *testing.T) *redis.Client {
	t.Helper()
	dsn := os.Getenv("VALKEY_URL")
	if dsn == "" {
		t.Skip("set VALKEY_URL to exercise the round-trip paths")
	}
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		t.Skipf("parse VALKEY_URL: %v", err)
	}
	c := redis.NewClient(opt)
	if err := c.Ping(context.Background()).Err(); err != nil {
		_ = c.Close()
		t.Skipf("valkey unreachable: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

type cached struct {
	Name string `json:"name"`
}

func TestKeysStayInsideTheirNamespace(t *testing.T) {
	if got, want := New(nil, "product").key("abc"), "cache:product:abc"; got != want {
		t.Errorf("key = %q, want %q", got, want)
	}
	if a, b := New(nil, "product").key("abc"), New(nil, "category").key("abc"); a == b {
		t.Errorf("two caches share key %q for the same suffix", a)
	}
}

func TestANilCacheIsAWorkingNoOpCache(t *testing.T) {
	// Deployments without Valkey wire nil, and CLI tooling builds services without one - every helper
	// has to answer rather than panic
	ctx := context.Background()
	for name, c := range map[string]*Cache{
		"a nil cache":            nil,
		"a cache with no client": New(nil, "product"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := GetJSON[cached](ctx, c, "k"); ok {
				t.Error("reported a hit with nothing behind it")
			}
			c.SetJSON(ctx, "k", cached{Name: "x"}, time.Minute)
			c.Del(ctx, "k")
			c.DelPattern(ctx, "*")
		})
	}
}

func TestAnOutageReadsAsAMiss(t *testing.T) {
	// Returning an error here would take the storefront down with Valkey instead of falling through to
	// Postgres, which is the whole reason this package is a cache and not a store
	ctx := context.Background()
	c := New(deadClient(), "product")

	if _, ok := GetJSON[cached](ctx, c, "k"); ok {
		t.Error("a dead client reported a hit")
	}
	c.SetJSON(ctx, "k", cached{Name: "x"}, time.Minute) // absorbed and logged, never returned
	c.Del(ctx, "k")
	c.DelPattern(ctx, "*")
}

func TestAPQFallsBackToFullQueries(t *testing.T) {
	// A miss makes gqlgen ask the client to resend the whole query - an error instead would surface as
	// PersistedQueryNotFound and break the request
	ctx := context.Background()
	for name, a := range map[string]*APQ{
		"no client":   NewAPQ(nil, "apq", time.Minute),
		"dead client": NewAPQ(deadClient(), "apq", time.Minute),
	} {
		t.Run(name, func(t *testing.T) {
			a.Add(ctx, "hash", "query { me }")
			if v, ok := a.Get(ctx, "hash"); ok || v != "" {
				t.Errorf("Get = (%q, %v), want a miss", v, ok)
			}
		})
	}
}

func TestRoundTripKeepsTheValueAndTheTTL(t *testing.T) {
	ctx := context.Background()
	client := liveClient(t)
	c := New(client, "test-roundtrip")
	suffix := "k-" + t.Name()
	t.Cleanup(func() { c.Del(ctx, suffix) })

	c.SetJSON(ctx, suffix, cached{Name: "tyre"}, 90*time.Second)
	got, ok := GetJSON[cached](ctx, c, suffix)
	if !ok || got.Name != "tyre" {
		t.Fatalf("read back (%+v, %v), want the stored value", got, ok)
	}
	ttl, err := client.PTTL(ctx, c.key(suffix)).Result()
	if err != nil {
		t.Fatalf("PTTL: %v", err)
	}
	if ttl <= 0 || ttl > 90*time.Second {
		// A key written without a TTL never expires, so a stale entry outlives every deploy
		t.Errorf("ttl = %v, want it inside the 90s window", ttl)
	}

	c.Del(ctx, suffix)
	if _, ok := GetJSON[cached](ctx, c, suffix); ok {
		t.Error("the key survived Del")
	}
}

func TestInvalidatingOneNamespaceLeavesTheOthers(t *testing.T) {
	// Two services caching the same entity id under the same suffix is the normal case, so a Del that
	// reached across namespaces would drop a neighbour's entry on every write
	ctx := context.Background()
	client := liveClient(t)
	mine, theirs := New(client, "test-mine"), New(client, "test-theirs")
	suffix := "shared-id"
	t.Cleanup(func() { mine.Del(ctx, suffix); theirs.Del(ctx, suffix) })

	mine.SetJSON(ctx, suffix, cached{Name: "mine"}, time.Minute)
	theirs.SetJSON(ctx, suffix, cached{Name: "theirs"}, time.Minute)

	mine.Del(ctx, suffix)
	if _, ok := GetJSON[cached](ctx, mine, suffix); ok {
		t.Error("my key survived my own Del")
	}
	got, ok := GetJSON[cached](ctx, theirs, suffix)
	if !ok || got.Name != "theirs" {
		t.Errorf("the neighbour's key = (%+v, %v), want it untouched", got, ok)
	}
}

func TestDelWithoutSuffixesIsNotAWipe(t *testing.T) {
	// Callers build the suffix list from a loop, and an empty one means "nothing changed" - issuing a
	// DEL with no keys, or worse a pattern, would be an accidental namespace flush
	ctx := context.Background()
	c := New(liveClient(t), "test-nodel")
	suffix := "keep-me"
	t.Cleanup(func() { c.Del(ctx, suffix) })

	c.SetJSON(ctx, suffix, cached{Name: "keep"}, time.Minute)
	c.Del(ctx)
	if _, ok := GetJSON[cached](ctx, c, suffix); !ok {
		t.Error("an empty Del removed the entry")
	}
}

func TestASchemaDriftedEntryIsDroppedRatherThanReturned(t *testing.T) {
	// After a deploy changes a cached struct, the old JSON no longer decodes - handing back the zero
	// value would serve empty products, and keeping the entry would do it until the TTL ran out
	ctx := context.Background()
	client := liveClient(t)
	c := New(client, "test-drift")
	suffix := "drifted"
	t.Cleanup(func() { c.Del(ctx, suffix) })

	c.SetJSON(ctx, suffix, []string{"not", "an", "object"}, time.Minute)
	if _, ok := GetJSON[cached](ctx, c, suffix); ok {
		t.Fatal("an undecodable entry reported a hit")
	}
	n, err := client.Exists(ctx, c.key(suffix)).Result()
	if err != nil {
		t.Fatalf("EXISTS: %v", err)
	}
	if n != 0 {
		t.Error("the undecodable entry was left in place, so every read until the TTL misses the same way")
	}
}

func TestDelPatternClearsOnlyItsOwnNamespace(t *testing.T) {
	ctx := context.Background()
	client := liveClient(t)
	mine, theirs := New(client, "test-sweep"), New(client, "test-keep")
	t.Cleanup(func() { mine.DelPattern(ctx, "*"); theirs.DelPattern(ctx, "*") })

	for _, suffix := range []string{"a", "b", "c"} {
		mine.SetJSON(ctx, suffix, cached{Name: suffix}, time.Minute)
		theirs.SetJSON(ctx, suffix, cached{Name: suffix}, time.Minute)
	}
	mine.DelPattern(ctx, "*")

	for _, suffix := range []string{"a", "b", "c"} {
		if _, ok := GetJSON[cached](ctx, mine, suffix); ok {
			t.Errorf("%q survived the sweep of its own namespace", suffix)
		}
		if _, ok := GetJSON[cached](ctx, theirs, suffix); !ok {
			t.Errorf("%q was swept out of the neighbouring namespace", suffix)
		}
	}
}

func TestAPQRoundTrip(t *testing.T) {
	ctx := context.Background()
	client := liveClient(t)
	a := NewAPQ(client, "test-apq", time.Minute)
	const hash = "sha256:test"
	t.Cleanup(func() { client.Del(ctx, "test-apq:"+hash) })

	a.Add(ctx, hash, "query { me }")
	got, ok := a.Get(ctx, hash)
	if !ok || got != "query { me }" {
		t.Fatalf("Get = (%q, %v), want the registered query", got, ok)
	}
	if _, ok := a.Get(ctx, "sha256:never-registered"); ok {
		t.Error("an unregistered hash reported a hit")
	}
}
