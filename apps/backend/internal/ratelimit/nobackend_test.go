package ratelimit

import (
	"context"
	"testing"
	"time"
)

// A nil client means VALKEY_URL is unset - a supported deployment, and failing open here would ship an unthrottled login endpoint
func TestRedisLimiterWithoutBackendStillEnforces(t *testing.T) {
	l := NewRedis(nil, 3, time.Minute)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		if !l.Allow(ctx, "1.2.3.4") {
			t.Fatalf("call %d denied, want allowed (budget is 3)", i)
		}
	}
	if l.Allow(ctx, "1.2.3.4") {
		t.Fatal("4th call allowed: limiter failed open without a backend")
	}
	if !l.Allow(ctx, "5.6.7.8") {
		t.Fatal("a different key was denied; buckets are not per-key")
	}
}

func TestTakeWithoutBackendReportsCounts(t *testing.T) {
	l := NewRedis(nil, 10, time.Minute)
	ctx := context.Background()

	n, ok := l.Take(ctx, "k")
	if !ok || n != 1 {
		t.Fatalf("first Take = (%d, %v), want (1, true)", n, ok)
	}
	n, ok = l.Take(ctx, "k")
	if !ok || n != 2 {
		t.Fatalf("second Take = (%d, %v), want (2, true)", n, ok)
	}
	// Throttling callers scale their response to the count, so ok=false would read as "no pressure" and disable backpressure
}

func TestNilLimiterIsInert(t *testing.T) {
	var l *RedisLimiter
	if n, ok := l.Take(context.Background(), "k"); ok || n != 0 {
		t.Fatalf("nil limiter Take = (%d, %v), want (0, false)", n, ok)
	}
}

func TestAllowNZeroRateDisablesCheck(t *testing.T) {
	l := NewRedis(nil, 1, time.Minute)
	ctx := context.Background()
	// rate <= 0 is the documented off switch, and it must stay off even though the fallback is now doing the counting
	for i := range 5 {
		if !l.AllowN(ctx, "k", 0) {
			t.Fatalf("AllowN with rate 0 denied on call %d", i+1)
		}
	}
}
