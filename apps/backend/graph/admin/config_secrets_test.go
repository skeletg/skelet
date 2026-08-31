package admin

import (
	"context"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// The split this guards: config.manage covers operational settings, config.manage_secrets covers
// credentials - the helper must fire ONLY for secret-typed paths so the extra grant never taxes
// ordinary settings work
func TestRequireSecretWritePermissionFiresOnlyForSecrets(t *testing.T) {
	// registered by plugins or core init - pick one of each type from the live registry so the test
	// tracks reality instead of a fixture
	var secretPath, plainPath string
	for _, d := range config.AllDefinitions() {
		if d.Type == config.TypeSecret && secretPath == "" {
			secretPath = d.Path
		}
		if d.Type == config.TypeString && plainPath == "" {
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
}
