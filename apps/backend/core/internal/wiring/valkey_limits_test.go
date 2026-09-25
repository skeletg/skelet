package wiring

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx/fxtest"
)

func TestRateLimitCountersGetTheirOwnValkeyWhenOneIsNamed(t *testing.T) {
	primary := redis.NewClient(&redis.Options{Addr: "valkey:6379"})
	cache := CacheRedis{redis.NewClient(&redis.Options{Addr: "cache:6379"})}
	t.Cleanup(func() { _ = cache.Close(); _ = primary.Close() })

	shared, err := newValkeyLimits(fxtest.NewLifecycle(t), Config{ValkeyCacheURL: "redis://cache:6379"}, primary, cache)
	if err != nil || shared.Client != cache.Client {
		t.Fatalf("with no limits URL the counters use %v, %v, want the cache client", shared.Client, err)
	}
	own, err := newValkeyLimits(fxtest.NewLifecycle(t), Config{ValkeyCacheURL: "redis://cache:6379", ValkeyLimitsURL: "redis://limits:6379"}, primary, cache)
	if err != nil || own.Client == cache.Client || own.Options().Addr != "limits:6379" {
		t.Fatalf("a named limits URL gave %v, %v, want a client of its own on limits:6379", own.Client, err)
	}
	_ = own.Close()
	onPrimary, err := newValkeyLimits(fxtest.NewLifecycle(t), Config{ValkeyURL: "redis://valkey:6379", ValkeyCacheURL: "redis://cache:6379", ValkeyLimitsURL: "redis://valkey:6379"}, primary, cache)
	if err != nil || onPrimary.Client != primary {
		t.Fatalf("a limits URL naming the primary gave %v, %v, want the primary client", onPrimary.Client, err)
	}
}
