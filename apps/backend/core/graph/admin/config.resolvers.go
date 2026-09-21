package admin

import (
	"context"
	"slices"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// The permission follows the scope being written - default scope needs the platform-baseline config.manage_defaults, channel scope needs config.manage on that channel
func (r *mutationResolver) SetConfigValue(ctx context.Context, path string, value string, scope *model.ConfigScopeInput) (*model.ConfigValue, error) {
	configScope := scopeFromInput(scope)
	if err := requireConfigScopePermission(ctx, r.AdminSvc, configWriteSlug(configScope), configScope); err != nil {
		return nil, err
	}
	if err := requireSecretWritePermission(ctx, r.AdminSvc, path, configScope); err != nil {
		return nil, err
	}
	entry, err := r.ConfigSvc.Set(ctx, path, value, configScope)
	if err != nil {
		return nil, err
	}
	return toConfigValueModel(ctx, entry), nil
}

// Deletes the row rather than writing a default, so the next read falls back to the default-scope row or the registry default
func (r *mutationResolver) ResetConfigValue(ctx context.Context, path string, scope *model.ConfigScopeInput) (*model.ConfigValue, error) {
	configScope := scopeFromInput(scope)
	if err := requireConfigScopePermission(ctx, r.AdminSvc, configWriteSlug(configScope), configScope); err != nil {
		return nil, err
	}
	// clearing a secret is as operational as writing one - a wiped webhook secret turns signature verification off
	if err := requireSecretWritePermission(ctx, r.AdminSvc, path, configScope); err != nil {
		return nil, err
	}
	entry, err := r.ConfigSvc.Reset(ctx, path, configScope)
	if err != nil {
		return nil, err
	}
	return toConfigValueModel(ctx, entry), nil
}

func (r *queryResolver) Config(ctx context.Context, scope *model.ConfigScopeInput) ([]*model.ConfigValue, error) {
	configScope := scopeFromInput(scope)
	if err := requireConfigScopePermission(ctx, r.AdminSvc, "config.view", configScope); err != nil {
		return nil, err
	}
	entries, err := r.ConfigSvc.ListAll(ctx, configScope)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ConfigValue, len(entries))
	for i, e := range entries {
		out[i] = toConfigValueModel(ctx, e)
	}
	return out, nil
}

func (r *queryResolver) ConfigSections(ctx context.Context, scope *model.ConfigScopeInput) ([]*model.ConfigSection, error) {
	configScope := scopeFromInput(scope)
	if err := requireConfigScopePermission(ctx, r.AdminSvc, "config.view", configScope); err != nil {
		return nil, err
	}
	entries, err := r.ConfigSvc.ListAll(ctx, configScope)
	if err != nil {
		return nil, err
	}
	if !r.AISvc.Enabled() { // AI_ENABLED=false hides Settings -> AI entirely, since the in-app toggle would be a lie
		entries = slices.DeleteFunc(entries, func(e config.Entry) bool {
			return strings.HasPrefix(e.Definition.Path, "ai/")
		})
	}
	return buildConfigSections(ctx, entries), nil
}

// An unknown path is a GraphQL error rather than a null - it's a developer bug, not a runtime condition worth hiding
func (r *queryResolver) ConfigValue(ctx context.Context, path string, scope *model.ConfigScopeInput) (*model.ConfigValue, error) {
	configScope := scopeFromInput(scope)
	if err := requireConfigScopePermission(ctx, r.AdminSvc, "config.view", configScope); err != nil {
		return nil, err
	}
	entry, err := r.ConfigSvc.Get(ctx, path, configScope)
	if err != nil {
		return nil, err
	}
	return toConfigValueModel(ctx, entry), nil
}
