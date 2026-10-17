package boot

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Every key the loader reads, set here so the container's own env can't leak into a case
func loadEnv(t *testing.T, encKey string) {
	t.Helper()
	_, pub, err := auth.GenerateCustomerKey()
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"APP_ENV": "development", "SIGNALS_DB_URL": "postgres://x@localhost/x", "JWT_SECRET": "",
		auth.EnvCustomerPublicKey: pub, "CUSTOMER_JWT_PUBLIC_KEY_PREVIOUS": "",
		"SIGNALS_SERVICE_SECRET": strings.Repeat("s", 40), "SIGNALS_SERVICE_SECRET_PREVIOUS": "",
		"EMAIL_LINK_SECRET": strings.Repeat("e", 40), "EMAIL_LINK_SECRET_PREVIOUS": "",
		EnvEncryptionKey: encKey,
	} {
		t.Setenv(k, v)
	}
}

func TestTheEncryptionKeyIsOptionalButNeverMalformed(t *testing.T) {
	loadEnv(t, "")
	c, err := Load()
	if err != nil || c.Secrets != nil {
		t.Fatalf("no key: secrets=%v err=%v, want plaintext dev and no error", c.Secrets, err)
	}
	loadEnv(t, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if c, err = Load(); err != nil || c.Secrets == nil {
		t.Fatalf("valid key: secrets=%v err=%v", c.Secrets, err)
	}
	loadEnv(t, base64.StdEncoding.EncodeToString(make([]byte, 16)))
	if _, err = Load(); err == nil || !strings.Contains(err.Error(), EnvEncryptionKey) {
		t.Fatalf("short key: err = %v, want a refusal naming %s", err, EnvEncryptionKey)
	}
}
