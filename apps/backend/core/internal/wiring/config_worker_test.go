package wiring

import (
	"os"
	"testing"
)

// loadBase stays rule-free so an entrypoint picks its own - cmd/server signs admin tokens with the JWT secret, so it demands one
func TestJWTSecretIsRequiredByTheServer(t *testing.T) {
	os.Unsetenv("JWT_SECRET")

	if _, err := loadBase(); err != nil {
		t.Fatalf("loadBase must not require JWT_SECRET: %v", err)
	}
	if _, err := LoadConfig(); err == nil {
		t.Error("LoadConfig must require JWT_SECRET")
	}
}
