package ai

import (
	"context"
	"log/slog"

	"github.com/skeletg/skelet/apps/backend/core/internal/aitrace"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// keyFor is the secret each address carries - pointed somewhere new, the old key would follow it there
var keyFor = map[string]string{cfgBaseURL: cfgAPIKey, cfgEmbedBaseURL: cfgEmbedAPIKey, aitrace.HostPath: aitrace.SecretKeyPath}

// GuardSecrets clears a provider's key once its address moves, so the settings show it needs entering again. It's only
// the tidy-up: every call checks config.BoundSecret itself, so the key never reaches the new address even before this runs
func GuardSecrets(cfg *config.Service) {
	cfg.OnChange(func(ctx context.Context, def config.Definition) {
		key, ok := keyFor[def.Path]
		if !ok {
			return
		}
		k, err := cfg.Get(ctx, key, config.DefaultScope)
		if err != nil || k.IsDefault {
			return
		}
		address, err := cfg.Get(ctx, def.Path, config.DefaultScope)
		if err != nil || config.BoundSecret(k, address) != "" {
			return
		}
		if _, err := cfg.Reset(ctx, key, config.DefaultScope); err != nil {
			slog.ErrorContext(ctx, "ai: clearing a key after its address changed failed - it isn't sent there either way", "address", def.Path, "err", err)
			return
		}
		slog.WarnContext(ctx, "ai: the address changed, so its key was cleared - enter it again for the new address", "address", def.Path, "key", key)
	})
}
