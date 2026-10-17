package wiring

import (
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// loadBase stays rule-free so an entrypoint picks its own - cmd/server signs sessions, so it demands the keys
func TestSessionKeysAreRequiredByTheServer(t *testing.T) {
	seed, _, _ := auth.GenerateCustomerKey()
	full := map[string]string{
		"ADMIN_JWT_SECRET": strings.Repeat("a", 32), "CUSTOMER_JWT_PRIVATE_KEY": seed,
		"EMAIL_LINK_SECRET": strings.Repeat("e", 32), "REVIEW_VOTER_TOKEN_SECRET": strings.Repeat("v", 32),
	}
	set := func(env map[string]string) {
		for _, k := range []string{"JWT_SECRET", "ADMIN_JWT_SECRET", "CUSTOMER_JWT_PRIVATE_KEY", "EMAIL_LINK_SECRET", "REVIEW_VOTER_TOKEN_SECRET"} {
			t.Setenv(k, env[k])
		}
	}
	set(nil)
	if _, err := loadBase(); err != nil {
		t.Fatalf("loadBase must not require session keys: %v", err)
	}
	set(full)
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig with every key: %v", err)
	}
	for missing := range full {
		env := maps.Clone(full)
		delete(env, missing)
		set(env)
		if _, err := LoadConfig(); err == nil {
			t.Errorf("LoadConfig without %s booted", missing)
		}
	}
	env := maps.Clone(full)
	env["JWT_SECRET"] = strings.Repeat("j", 32)
	set(env)
	if _, err := LoadConfig(); !errors.Is(err, auth.ErrRetiredJWTSecret) {
		t.Errorf("LoadConfig with the retired JWT_SECRET = %v, want ErrRetiredJWTSecret", err)
	}
}
