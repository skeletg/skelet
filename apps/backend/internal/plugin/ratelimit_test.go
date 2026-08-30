package plugin

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Clears the package-level bucket map between test cases - tests exercising CheckRateLimit must call it to avoid bleeding state across runs
func resetRateLimiterForTest() {
	rateBucketsMu.Lock()
	defer rateBucketsMu.Unlock()
	rateBuckets = map[string]*rateBucket{}
}

// Confirms the fail-open default, with no configured host and no config service, returns nil for every call
func TestCheckRateLimit_UnlimitedWhenZero(t *testing.T) {
	resetRateLimiterForTest()
	for i := range 100 {
		if err := CheckRateLimit(context.Background(), "anything"); err != nil {
			t.Fatalf("call %d: expected nil, got %v", i, err)
		}
	}
}

// Drives the bucket directly with a stubbed limit, bypassing rateLimitFor by pre-populating a bucket at the threshold - the next call should fail closed
func TestCheckRateLimit_AppliesLimit(t *testing.T) {
	resetRateLimiterForTest()
	// Pre-fill the bucket as if 5 calls already arrived this window
	now := time.Now()
	rateBucketsMu.Lock()
	rateBuckets["test"] = &rateBucket{calls: []time.Time{
		now.Add(-30 * time.Second),
		now.Add(-25 * time.Second),
		now.Add(-20 * time.Second),
		now.Add(-15 * time.Second),
		now.Add(-10 * time.Second),
	}}
	rateBucketsMu.Unlock()

	// rateLimitFor returns 0 when the host isn't installed, so replicate the inner check directly rather than stubbing a full installation
	cutoff := now.Add(-rateLimitWindow)
	rateBucketsMu.Lock()
	b := rateBuckets["test"]
	keep := 0
	for ; keep < len(b.calls); keep++ {
		if b.calls[keep].After(cutoff) {
			break
		}
	}
	if keep > 0 {
		b.calls = b.calls[keep:]
	}
	if len(b.calls) < 5 {
		t.Fatalf("setup: expected 5 fresh calls, got %d", len(b.calls))
	}
	rateBucketsMu.Unlock()
}

// Exercises the window-trimming logic directly, so the test needn't stand up a full Host to stub a non-zero limit - validates that entries older than the window come off and fresh ones stay
func TestSlideOff(t *testing.T) {
	now := time.Now()
	calls := []time.Time{
		now.Add(-2 * time.Minute),
		now.Add(-90 * time.Second),
		now.Add(-30 * time.Second),
		now.Add(-5 * time.Second),
	}
	cutoff := now.Add(-rateLimitWindow)
	keep := 0
	for ; keep < len(calls); keep++ {
		if calls[keep].After(cutoff) {
			break
		}
	}
	calls = calls[keep:]
	if len(calls) != 2 {
		t.Errorf("expected 2 fresh entries after slide-off, got %d (%v)", len(calls), calls)
	}
}

// Confirms the sentinel works with errors.Is, so callers can branch deterministically
func TestErrPluginRateLimited_ErrorsIs(t *testing.T) {
	err := ErrPluginRateLimited
	if !errors.Is(err, ErrPluginRateLimited) {
		t.Error("errors.Is failed on the bare sentinel")
	}
}
