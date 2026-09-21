package auth

import (
	"context"
	"testing"
	"time"
)

// Without Valkey the lockout must still happen, per-process - a fail-open throttle plus a fail-open rate limiter would leave slow credential guessing completely unopposed
func TestLoginThrottleWithoutBackendStillLocks(t *testing.T) {
	th := NewLoginThrottle(nil, 3, time.Minute, time.Minute)
	ctx := context.Background()
	const email = "victim@example.com"

	if th.Locked(ctx, "admin", email) {
		t.Fatal("locked before any failures")
	}
	for range 3 {
		th.RecordFailure(ctx, "admin", email)
	}
	if !th.Locked(ctx, "admin", email) {
		t.Fatal("not locked after hitting the threshold: throttle failed open")
	}

	if th.Locked(ctx, "customer", email) { // scope and email both partition the lock
		t.Error("admin failures locked the customer scope")
	}
	if th.Locked(ctx, "admin", "someone-else@example.com") {
		t.Error("one account's failures locked another")
	}

	th.Clear(ctx, "admin", email) // a successful login clears it
	if th.Locked(ctx, "admin", email) {
		t.Error("still locked after Clear")
	}
}

func TestLoginThrottleWithoutBackendIgnoresBlankEmail(t *testing.T) {
	th := NewLoginThrottle(nil, 1, time.Minute, time.Minute)
	ctx := context.Background()
	th.RecordFailure(ctx, "admin", "   ")
	if th.Locked(ctx, "admin", "   ") {
		t.Fatal("blank email produced a lock")
	}
}

func TestNilThrottleIsInert(t *testing.T) {
	var th *LoginThrottle
	if th.Locked(context.Background(), "admin", "a@b.c") {
		t.Fatal("nil throttle reported locked")
	}
	th.RecordFailure(context.Background(), "admin", "a@b.c") // must not panic
	th.Clear(context.Background(), "admin", "a@b.c")
}
