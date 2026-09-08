package wiring

import (
	"os"
	"testing"
)

// loadBase stays rule-free so an entrypoint picks its own, and both now demand a JWT secret - cmd/server signs admin tokens with it and cmd/signals verifies service tokens
func TestJWTSecretIsRequiredByBothEntrypoints(t *testing.T) {
	os.Setenv("SIGNALS_DB_URL", "postgres://x/y")
	os.Unsetenv("JWT_SECRET")

	if _, err := loadBase(); err != nil {
		t.Fatalf("loadBase must not require JWT_SECRET: %v", err)
	}
	if _, err := LoadConfig(); err == nil {
		t.Error("LoadConfig must require JWT_SECRET")
	}
	// Without it every service token fails to verify and the Signals API refuses silently, so this has to fail at boot
	if _, err := LoadSignalsConfig(); err == nil {
		t.Error("LoadSignalsConfig must require JWT_SECRET now that it serves an authenticated API")
	}

	os.Setenv("JWT_SECRET", "test-secret")
	c, err := LoadSignalsConfig()
	if err != nil {
		t.Fatalf("LoadSignalsConfig: %v", err)
	}
	if c.MaxConns != 4 {
		t.Fatalf("signals primary pool should default to 4, got %d", c.MaxConns)
	}
}
