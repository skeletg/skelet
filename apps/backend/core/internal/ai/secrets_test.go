package ai

import (
	"context"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type memRepo map[string]config.StoredValue

func (m memRepo) Upsert(_ context.Context, scope config.Scope, path, value string) (config.StoredValue, error) {
	v := config.StoredValue{Scope: scope, Path: path, Value: value, UpdatedAt: time.Now()}
	m[path] = v
	return v, nil
}
func (m memRepo) Delete(_ context.Context, _ config.Scope, path string) error {
	delete(m, path)
	return nil
}
func (m memRepo) GetCandidates(_ context.Context, _ config.Scope, path string) ([]config.StoredValue, error) {
	if v, ok := m[path]; ok {
		return []config.StoredValue{v}, nil
	}
	return nil, nil
}
func (m memRepo) ListInScopes(context.Context, config.Scope) ([]config.StoredValue, error) {
	return nil, nil
}
func (m memRepo) ListAll(context.Context) ([]config.StoredValue, error) { return nil, nil }

func TestMovingAnAddressClearsTheKeyItCarried(t *testing.T) {
	ctx := context.Background()
	repo := memRepo{}
	cfg := config.NewService(repo)
	GuardSecrets(cfg)
	if _, err := cfg.Set(ctx, cfgEmbedAPIKey, "secret-1", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Set(ctx, cfgEmbedBaseURL, "https://api.voyageai.com/v1", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo[cfgEmbedAPIKey]; !ok {
		t.Fatal("a key saved with the address in the same save was cleared")
	}
	v := repo[cfgEmbedAPIKey]
	v.UpdatedAt = time.Now().Add(-time.Hour)
	repo[cfgEmbedAPIKey] = v
	if _, err := cfg.Set(ctx, cfgEmbedBaseURL, "https://elsewhere.example/v1", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo[cfgEmbedAPIKey]; ok {
		t.Error("the address moved and the hour-old key would have followed it")
	}
}

func aged(repo memRepo, path string) {
	v := repo[path]
	v.UpdatedAt = time.Now().Add(-time.Hour)
	repo[path] = v
}

func TestAKeyIsNeverSentToAnAddressItWasntEnteredFor(t *testing.T) {
	ctx := context.Background()
	repo := memRepo{}
	cfg := config.NewService(repo) // no GuardSecrets, so nothing tidies the key away - the read itself must refuse it
	svc := NewService(cfg)
	if _, err := cfg.Set(ctx, cfgAPIKey, "secret-1", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	if got := svc.providerConfig(ctx).apiKey; got != "secret-1" {
		t.Fatalf("the built-in address got %q", got)
	}
	aged(repo, cfgAPIKey)
	if _, err := cfg.Set(ctx, cfgBaseURL, "https://elsewhere.example/v1", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	if got := svc.providerConfig(ctx).apiKey; got != "" {
		t.Errorf("the moved address got %q - want no key until one is entered for it", got)
	}
	if _, err := cfg.Set(ctx, cfgEmbedAPIKey, "embed-1", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	aged(repo, cfgEmbedAPIKey)
	if _, err := cfg.Set(ctx, cfgEmbedBaseURL, "https://elsewhere.example/v1", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	if got := svc.Embedder().cfg(ctx).key; got != "" {
		t.Errorf("the embeddings' moved address got %q", got)
	}
}

func TestAKeyFromTheEnvironmentOnlyGoesToTheProvidersOwnAddress(t *testing.T) {
	ctx := context.Background()
	t.Setenv("ANTHROPIC_API_KEY", "env-key")
	t.Setenv("AI_API_KEY", "")
	cfg := config.NewService(memRepo{})
	svc := NewService(cfg)
	if got := svc.providerConfig(ctx).apiKey; got != "env-key" {
		t.Fatalf("the built-in address got %q", got)
	}
	if _, err := cfg.Set(ctx, cfgBaseURL, "https://elsewhere.example/v1", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	if got := svc.providerConfig(ctx).apiKey; got != "" {
		t.Errorf("an address set in the settings got the environment's key %q", got)
	}
}

func TestSavingTheSameAddressAgainKeepsItsKey(t *testing.T) {
	ctx := context.Background()
	repo := memRepo{}
	cfg := config.NewService(repo)
	GuardSecrets(cfg)
	for _, set := range [][2]string{{cfgBaseURL, "https://gateway.example/v1"}, {cfgAPIKey, "secret-1"}} {
		if _, err := cfg.Set(ctx, set[0], set[1], config.DefaultScope); err != nil {
			t.Fatal(err)
		}
	}
	aged(repo, cfgAPIKey)
	aged(repo, cfgBaseURL)
	if _, err := cfg.Set(ctx, cfgBaseURL, "https://gateway.example/v1", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	if got := NewService(cfg).providerConfig(ctx).apiKey; got != "secret-1" {
		t.Errorf("re-saving the address unchanged left the key %q", got)
	}
}
