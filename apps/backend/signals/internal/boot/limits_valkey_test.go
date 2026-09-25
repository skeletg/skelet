package boot

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx/fxtest"
)

func TestRateLimitCountersGetTheirOwnValkeyWhenOneIsNamed(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	cache := redis.NewClient(&redis.Options{Addr: "cache:6379"})
	defer cache.Close()

	shared, err := newLimitsValkey(lc, Config{ValkeyURL: "redis://cache:6379"}, cache)
	if err != nil || shared.Client != cache {
		t.Fatalf("no limits url = %v, %v, want the cache client", shared.Client, err)
	}
	own, err := newLimitsValkey(lc, Config{ValkeyURL: "redis://cache:6379", ValkeyLimitsURL: "redis://limits:6379"}, cache)
	if err != nil || own.Client == cache || own.Options().Addr != "limits:6379" {
		t.Fatalf("named limits url = %v, %v, want its own client on limits:6379 - counters on the cache get evicted and lift every limit", own.Client, err)
	}
}
