package signalsclient

import (
	"context"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// The suite's settings store over the contract - core administers these, so the admin gate runs in its own resolver first
// Only two core paths read one, and the config service caches in front

type settingsRepo struct{ client *Client }

func NewSettingsRepo(c *Client) config.Repository {
	return settingsRepo{client: c}
}

func (r settingsRepo) Upsert(ctx context.Context, scope config.Scope, path, value string) (config.StoredValue, error) {
	v, err := r.client.UpsertSetting(ctx, wireSettingScope(scope), path, value)
	if err != nil {
		return config.StoredValue{}, err
	}
	return storedValue(v), nil
}

func (r settingsRepo) Delete(ctx context.Context, scope config.Scope, path string) error {
	return r.client.DeleteSetting(ctx, wireSettingScope(scope), path)
}

func (r settingsRepo) GetCandidates(ctx context.Context, scope config.Scope, path string) ([]config.StoredValue, error) {
	vs, err := r.client.SettingCandidates(ctx, wireSettingScope(scope), path)
	return storedValues(vs), err
}

func (r settingsRepo) ListInScopes(ctx context.Context, scope config.Scope) ([]config.StoredValue, error) {
	vs, err := r.client.SettingsInScope(ctx, wireSettingScope(scope))
	return storedValues(vs), err
}

func (r settingsRepo) ListAll(ctx context.Context) ([]config.StoredValue, error) {
	vs, err := r.client.AllSettings(ctx)
	return storedValues(vs), err
}

func wireSettingScope(s config.Scope) SettingScope {
	return SettingScope{Kind: s.Kind, ChannelID: s.ChannelID}
}

func storedValue(v SettingValue) config.StoredValue {
	out := config.StoredValue{
		Scope: config.Scope{Kind: v.Scope.Kind, ChannelID: v.Scope.ChannelID},
		Path:  v.Path, Value: v.Value,
	}
	if v.UpdatedAt > 0 {
		out.UpdatedAt = time.Unix(v.UpdatedAt, 0).UTC()
	}
	return out
}

func storedValues(in []SettingValue) []config.StoredValue {
	out := make([]config.StoredValue, 0, len(in))
	for _, v := range in {
		out = append(out, storedValue(v))
	}
	return out
}
