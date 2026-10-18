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

func TestRateBucketAdmitsUpToTheLimit(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	b := &rateBucket{}
	for i := range 5 {
		if !b.admit(t0.Add(time.Duration(i)*time.Second), 5) {
			t.Fatalf("call %d refused under the limit", i)
		}
	}
	if b.admit(t0.Add(10*time.Second), 5) {
		t.Fatal("sixth call in the window admitted")
	}
	if b.cur != 5 {
		t.Errorf("a refused call consumed a slot: cur=%d", b.cur)
	}
}

// Last minute's calls still count for the part of it the window overlaps, so the boundary is no burst point
func TestRateBucketSlides(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	b := &rateBucket{}
	for range 10 {
		b.admit(t0.Add(50*time.Second), 10)
	}
	admitted := 0
	for range 10 {
		if b.admit(t0.Add(61*time.Second), 10) {
			admitted++
		}
	}
	if admitted > 1 {
		t.Errorf("admitted a burst of %d right after the boundary while last minute's 10 still fill the window", admitted)
	}
	if !b.admit(t0.Add(115*time.Second), 10) {
		t.Error("refused once most of last minute had slid out")
	}
	if !b.admit(t0.Add(5*time.Minute), 10) {
		t.Error("refused after the window fully expired")
	}
	if b.prev != 0 {
		t.Errorf("a stale window carried over: prev=%d", b.prev)
	}
}

// Confirms the sentinel works with errors.Is, so callers can branch deterministically
func TestErrPluginRateLimited_ErrorsIs(t *testing.T) {
	err := ErrPluginRateLimited
	if !errors.Is(err, ErrPluginRateLimited) {
		t.Error("errors.Is failed on the bare sentinel")
	}
}
