package wiring

import (
	"testing"
)

func TestRateLimitDefaultsToOff(t *testing.T) { // rate limiting is OFF unless asked for - the default a fresh install boots with, so it gets a test rather than a comment
	if RateLimitEnabled() {
		t.Fatal("RATE_LIMIT_ENABLED unset must read as off")
	}
	for _, key := range []string{
		"GENERAL_RATE_LIMIT_PER_MIN",
		"AUTH_RATE_LIMIT_PER_MIN",
		"FRONT_RATE_LIMIT_PER_MIN",
		"MEDIA_TRANSFORM_RATE_LIMIT_PER_MIN",
		"TOKEN_RATE_LIMIT_PER_MIN",
		"INGEST_RATE_LIMIT_PER_MIN",
	} {
		t.Setenv(key, "42") // a surface knob alone must not switch the layer on
		if got := RateLimitPerMin(key, 100); got != 0 {
			t.Errorf("RateLimitPerMin(%s) = %d with the layer off, want 0", key, got)
		}
	}
}

func TestRateLimitPerMinWhenEnabled(t *testing.T) { // the per-surface knobs only bite once the master switch is on, and each keeps its own default
	t.Setenv("RATE_LIMIT_ENABLED", "true")
	if got := RateLimitPerMin("SOME_RATE_LIMIT_PER_MIN", 300); got != 300 {
		t.Errorf("unset knob = %d, want the default 300", got)
	}
	t.Setenv("SOME_RATE_LIMIT_PER_MIN", "42")
	if got := RateLimitPerMin("SOME_RATE_LIMIT_PER_MIN", 300); got != 42 {
		t.Errorf("set knob = %d, want 42", got)
	}
}

func TestLoginThrottleDefaultsToNil(t *testing.T) { // a nil throttle is what disables the lockout, and every LoginThrottle method is nil-receiver safe
	if newLoginThrottle(nil) != nil {
		t.Fatal("login lockout must be nil with both switches unset")
	}
	if !newLoginThrottle(nil).Locked(t.Context(), "admin", "someone@example.com") == false {
		t.Fatal("a nil throttle must never report locked")
	}
	t.Setenv("LOGIN_LOCKOUT_ENABLED", "true")
	if newLoginThrottle(nil) == nil {
		t.Fatal("LOGIN_LOCKOUT_ENABLED=true must build a throttle")
	}
	t.Setenv("LOGIN_LOCKOUT_ENABLED", "")
	t.Setenv("RATE_LIMIT_ENABLED", "true")
	if newLoginThrottle(nil) == nil {
		t.Fatal("the master switch must turn the lockout on too")
	}
}
