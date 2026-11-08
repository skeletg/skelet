package boot

import (
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Every key the loader reads, set here so the container's own env can't leak into a case
func loadEnv(t *testing.T) {
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
	} {
		t.Setenv(k, v)
	}
}

func TestTheSuiteLoadsWithoutAKeyOfItsOwnToSealWith(t *testing.T) {
	loadEnv(t)
	if _, err := Load(); err != nil {
		t.Fatalf("a full environment = %v", err)
	}
	t.Setenv("SIGNALS_SERVICE_SECRET", "")
	if _, err := Load(); err == nil {
		t.Error("no service secret loaded anyway - the contract would refuse every call")
	}
}
