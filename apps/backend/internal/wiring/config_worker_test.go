package wiring

import (
	"os"
	"testing"
)

// The worker path must not inherit the server's JWT requirement
func TestLoadBaseSkipsJWTRule(t *testing.T) {
	os.Setenv("SIGNALS_DB_URL", "postgres://x/y")
	os.Unsetenv("JWT_SECRET")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig must still require JWT_SECRET")
	}
	if _, err := loadBase(); err != nil {
		t.Fatalf("loadBase must not require JWT_SECRET: %v", err)
	}
	c, err := LoadSignalsConfig()
	if err != nil {
		t.Fatalf("LoadSignalsConfig: %v", err)
	}
	if c.MaxConns != 4 {
		t.Fatalf("signals primary pool should default to 4, got %d", c.MaxConns)
	}
}
