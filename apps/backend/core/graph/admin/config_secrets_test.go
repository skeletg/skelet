package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// The split this guards: config.manage covers operational settings, config.manage_secrets covers credentials - the helper must fire ONLY for secret-typed paths so the extra grant never taxes ordinary settings work
func TestRequireSecretWritePermissionFiresOnlyForSecrets(t *testing.T) {
	// registered by plugins or core init - pick one of each type from the live registry so the test
	// tracks reality instead of a fixture
	var secretPath, plainPath string
	for _, d := range config.AllDefinitions() {
		if d.Type == config.TypeSecret && !d.SuperuserOnly && secretPath == "" {
			secretPath = d.Path
		}
		if d.Type == config.TypeString && !d.SuperuserOnly && plainPath == "" {
			plainPath = d.Path
		}
	}
	if secretPath == "" || plainPath == "" {
		t.Skip("registry carries no secret+string pair to compare")
	}

	if def, ok := config.Find(secretPath); !ok || def.Type != config.TypeSecret {
		t.Fatalf("Find(%q) lost the secret definition", secretPath)
	}
	// unknown and plain paths must not demand the secrets grant - the helper returns nil before any
	// permission check, which is what these assert by not needing a checker at all
	if err := requireSecretWritePermission(context.TODO(), nil, plainPath, config.Scope{}); err != nil {
		t.Errorf("plain path demanded the secrets grant: %v", err)
	}
	if err := requireSecretWritePermission(context.TODO(), nil, "no/such/path", config.Scope{}); err != nil {
		t.Errorf("unknown path demanded the secrets grant: %v", err)
	}
	for _, path := range []string{"general/admin_sign_in/oidc_issuer", "general/customer_sign_in/oidc_client_secret"} {
		if err := requireSecretWritePermission(context.TODO(), nil, path, config.Scope{}); err == nil {
			t.Errorf("%s was writable signed out", path)
		}
		admin := auth.WithAdmin(context.TODO(), &auth.Claims{AdminID: uuid.New()})
		if err := requireSecretWritePermission(admin, nil, path, config.Scope{}); apperr.CodeOf(err) != "CONFIG_SUPERUSER_ONLY" {
			t.Errorf("%s was writable by an admin who isn't a superuser: %v", path, err)
		}
		superuser := auth.WithAdmin(context.TODO(), &auth.Claims{AdminID: uuid.New(), IsSuperuser: true})
		if err := requireSecretWritePermission(superuser, nil, path, config.Scope{}); err != nil {
			t.Errorf("%s refused a superuser: %v", path, err)
		}
	}
}
