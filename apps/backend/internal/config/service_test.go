package config

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/secrets"
)

// An in-memory stand-in for the Postgres repository - keyed the same way the real table's unique
// index is, so the fallback behaviour under test is the same shape
type fakeRepo struct {
	rows      map[string]StoredValue
	upsertErr error
	deleteErr error
	listErr   error
	deletes   []string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[string]StoredValue{}} }

func (r *fakeRepo) Upsert(_ context.Context, scope Scope, path, value string) (StoredValue, error) {
	if r.upsertErr != nil {
		return StoredValue{}, r.upsertErr
	}
	sv := StoredValue{Scope: scope, Path: path, Value: value, UpdatedAt: time.Unix(1755300000, 0).UTC()}
	r.rows[scopeKey(scope, path)] = sv
	return sv, nil
}

func (r *fakeRepo) Delete(_ context.Context, scope Scope, path string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.deletes = append(r.deletes, FormatScope(scope)+" "+path)
	delete(r.rows, scopeKey(scope, path))
	return nil
}

func (r *fakeRepo) GetCandidates(_ context.Context, scope Scope, path string) ([]StoredValue, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]StoredValue, 0, 3)
	for _, sv := range r.rows {
		if sv.Path == path && candidateFor(sv.Scope, scope) {
			out = append(out, sv)
		}
	}
	return out, nil
}

// Mirrors the WHERE in GetConfigValueWithFallback / ListConfigValuesInScopes: the default row, plus
// this channel's rows whose locale is null or the requested one
func candidateFor(row, scope Scope) bool {
	if row.Kind == ScopeDefault {
		return row.ChannelID == nil && row.Locale == nil
	}
	if scope.Kind != ScopeChannel || row.Kind != ScopeChannel {
		return false
	}
	if !uuidPtrEqual(row.ChannelID, scope.ChannelID) {
		return false
	}
	return row.Locale == nil || (scope.Locale != nil && *row.Locale == *scope.Locale)
}

func (r *fakeRepo) ListInScopes(_ context.Context, scope Scope) ([]StoredValue, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]StoredValue, 0, len(r.rows))
	for _, sv := range r.rows {
		if candidateFor(sv.Scope, scope) {
			out = append(out, sv)
		}
	}
	return out, nil
}

func (r *fakeRepo) ListAll(_ context.Context) ([]StoredValue, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]StoredValue, 0, len(r.rows))
	for _, sv := range r.rows {
		out = append(out, sv)
	}
	return out, nil
}

// A registry holding just the paths the test needs, plus the service over a fresh fake repo
func newTestService(t *testing.T, defs ...Definition) (*Service, *fakeRepo) {
	t.Helper()
	isolateRegistries(t)
	RegisterSection(Section{Slug: "shop"})
	// Channel-scoped, since most of these tests write and resolve at channel scope - Set refuses a
	// channel row on a platform-wide knob, which TestSetRefusesAScopeNothingReads covers
	RegisterGroup(Group{Section: "shop", Slug: "general", Scoping: ScopingChannel})
	for _, d := range defs {
		Register(d)
	}
	repo := newFakeRepo()
	return NewService(repo), repo
}

func testCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	c, err := secrets.NewCipher(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	return c
}

func TestGetAndSetRejectUnknownPaths(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Get(ctx, "shop/general/ghost", DefaultScope); !errors.Is(err, ErrUnknownPath) {
		t.Errorf("Get on an unregistered path = %v, want ErrUnknownPath", err)
	}
	if _, err := svc.Set(ctx, "shop/general/ghost", "x", DefaultScope); !errors.Is(err, ErrUnknownPath) {
		t.Errorf("Set on an unregistered path = %v, want ErrUnknownPath", err)
	}
	if _, err := svc.Reset(ctx, "shop/general/ghost", DefaultScope); !errors.Is(err, ErrUnknownPath) {
		t.Errorf("Reset on an unregistered path = %v, want ErrUnknownPath", err)
	}
}

// Channel row beats default row beats the code-declared default - this chain is what makes a
// per-channel override actually override
func TestGetResolvesScopedThenDefaultThenRegistryDefault(t *testing.T) {
	svc, _ := newTestService(t, Definition{Path: "shop/general/name", Type: TypeString, Default: "Skelet"})
	ctx := context.Background()
	channelID := uuid.New()
	channel := Scope{Kind: ScopeChannel, ChannelID: &channelID}

	got, err := svc.Get(ctx, "shop/general/name", channel)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != "Skelet" || !got.IsDefault || got.Scope != DefaultScope {
		t.Errorf("with nothing stored = %+v, want the registry default at default scope", got)
	}
	if !got.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt = %v, want zero when the value is the registry default", got.UpdatedAt)
	}

	if _, err := svc.Set(ctx, "shop/general/name", "Global Store", DefaultScope); err != nil {
		t.Fatalf("Set at default scope: %v", err)
	}
	got, _ = svc.Get(ctx, "shop/general/name", channel)
	if got.Value != "Global Store" || got.IsDefault || got.Scope != DefaultScope {
		t.Errorf("with only a default row = %+v, want the stored default value", got)
	}

	if _, err := svc.Set(ctx, "shop/general/name", "Channel Store", channel); err != nil {
		t.Fatalf("Set at channel scope: %v", err)
	}
	got, _ = svc.Get(ctx, "shop/general/name", channel)
	if got.Value != "Channel Store" || got.IsDefault || got.Scope != channel {
		t.Errorf("with a channel row = %+v, want the channel value", got)
	}
	// the default scope still reads its own row, unaffected by the override
	got, _ = svc.Get(ctx, "shop/general/name", DefaultScope)
	if got.Value != "Global Store" {
		t.Errorf("default scope = %q, want the channel override invisible there", got.Value)
	}

	// clearing the channel row falls back one rung rather than to the registry default
	if _, err := svc.Reset(ctx, "shop/general/name", channel); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	got, _ = svc.Get(ctx, "shop/general/name", channel)
	if got.Value != "Global Store" || got.Scope != DefaultScope {
		t.Errorf("after Reset = %+v, want the stored default back", got)
	}
}

func TestSetNormalizesBeforeStoring(t *testing.T) {
	svc, repo := newTestService(t,
		Definition{Path: "shop/general/per_page", Type: TypeInt, Default: "12"},
		Definition{Path: "shop/general/accent", Type: TypeColor},
	)
	ctx := context.Background()

	entry, err := svc.Set(ctx, "shop/general/per_page", "024", DefaultScope)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if entry.Value != "24" {
		t.Errorf("returned value = %q, want the normalized %q", entry.Value, "24")
	}
	if stored := repo.rows[scopeKey(DefaultScope, "shop/general/per_page")].Value; stored != "24" {
		t.Errorf("stored value = %q, want the normalized %q", stored, "24")
	}

	if _, err := svc.Set(ctx, "shop/general/accent", "#AABBCC", DefaultScope); err != nil {
		t.Fatalf("Set color: %v", err)
	}
	if stored := repo.rows[scopeKey(DefaultScope, "shop/general/accent")].Value; stored != "#aabbcc" {
		t.Errorf("stored color = %q, want it lowercased", stored)
	}
}

// A rejected value must not reach the repository at all, or the admin sees an error and the row
// changes anyway
func TestSetRejectsInvalidValuesBeforeWriting(t *testing.T) {
	svc, repo := newTestService(t, Definition{Path: "shop/general/per_page", Type: TypeInt, Default: "12"})
	ctx := context.Background()
	if _, err := svc.Set(ctx, "shop/general/per_page", "lots", DefaultScope); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("Set with a bad int = %v, want ErrInvalidValue", err)
	}
	if len(repo.rows) != 0 {
		t.Errorf("repo holds %d rows after a rejected Set, want 0", len(repo.rows))
	}
}

func TestSetAndGetRejectInvalidScopes(t *testing.T) {
	svc, _ := newTestService(t, Definition{Path: "shop/general/name", Type: TypeString})
	ctx := context.Background()
	channelID := uuid.New()
	bad := Scope{Kind: ScopeDefault, ChannelID: &channelID}

	if _, err := svc.Get(ctx, "shop/general/name", bad); !errors.Is(err, ErrInvalidScope) {
		t.Errorf("Get with a channel-carrying default scope = %v, want ErrInvalidScope", err)
	}
	if _, err := svc.Set(ctx, "shop/general/name", "x", bad); !errors.Is(err, ErrInvalidScope) {
		t.Errorf("Set with a channel-carrying default scope = %v, want ErrInvalidScope", err)
	}
	if _, err := svc.ListAll(ctx, bad); !errors.Is(err, ErrInvalidScope) {
		t.Errorf("ListAll with a bad scope = %v, want ErrInvalidScope", err)
	}
}

// Typed getters are what callers on hot paths use, so a stored value that no longer parses has to
// surface as ErrInvalidValue rather than a silent zero
func TestTypedGetters(t *testing.T) {
	svc, _ := newTestService(t,
		Definition{Path: "shop/general/per_page", Type: TypeInt, Default: "12"},
		Definition{Path: "shop/general/enabled", Type: TypeBool, Default: "true"},
		Definition{Path: "shop/general/shape", Type: TypeJSON, Default: `{"a":1}`},
		Definition{Path: "shop/general/name", Type: TypeString, Default: "Skelet"},
	)
	ctx := context.Background()

	if n, err := svc.GetInt(ctx, "shop/general/per_page", DefaultScope); err != nil || n != 12 {
		t.Errorf("GetInt = (%d, %v), want (12, nil)", n, err)
	}
	if b, err := svc.GetBool(ctx, "shop/general/enabled", DefaultScope); err != nil || !b {
		t.Errorf("GetBool = (%v, %v), want (true, nil)", b, err)
	}
	if s, err := svc.GetString(ctx, "shop/general/name", DefaultScope); err != nil || s != "Skelet" {
		t.Errorf("GetString = (%q, %v), want (\"Skelet\", nil)", s, err)
	}
	var shape struct {
		A int `json:"a"`
	}
	if err := svc.GetJSON(ctx, "shop/general/shape", DefaultScope, &shape); err != nil || shape.A != 1 {
		t.Errorf("GetJSON = (%+v, %v), want a=1", shape, err)
	}

	// a registry default that doesn't match its declared type only bites at read time
	if _, err := svc.GetInt(ctx, "shop/general/name", DefaultScope); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("GetInt on a string value = %v, want ErrInvalidValue", err)
	}
	if _, err := svc.GetBool(ctx, "shop/general/name", DefaultScope); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("GetBool on a string value = %v, want ErrInvalidValue", err)
	}
	if err := svc.GetJSON(ctx, "shop/general/name", DefaultScope, &shape); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("GetJSON on a non-JSON value = %v, want ErrInvalidValue", err)
	}
	if _, err := svc.GetInt(ctx, "shop/general/ghost", DefaultScope); !errors.Is(err, ErrUnknownPath) {
		t.Errorf("GetInt on an unknown path = %v, want ErrUnknownPath", err)
	}
}

// Gateway keys must not sit in the table as plaintext, but the caller always gets plaintext back -
// the admin resolver's "is it set" check would otherwise read ciphertext
func TestSecretsAreEncryptedAtRestAndDecryptedOnRead(t *testing.T) {
	svc, repo := newTestService(t, Definition{Path: "shop/general/api_key", Type: TypeSecret})
	svc.SetCipher(testCipher(t))
	ctx := context.Background()

	entry, err := svc.Set(ctx, "shop/general/api_key", "sk_live_supersecret", DefaultScope)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if entry.Value != "sk_live_supersecret" {
		t.Errorf("returned value = %q, want the plaintext", entry.Value)
	}
	stored := repo.rows[scopeKey(DefaultScope, "shop/general/api_key")].Value
	if stored == "sk_live_supersecret" {
		t.Fatal("the secret was stored as plaintext")
	}
	if !secrets.IsEncrypted(stored) {
		t.Fatalf("stored value = %q, want it carry the encryption prefix", stored)
	}

	got, err := svc.Get(ctx, "shop/general/api_key", DefaultScope)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != "sk_live_supersecret" {
		t.Errorf("Get = %q, want the decrypted plaintext", got.Value)
	}
}

// Dev runs without a key, and turning the key on later must not break the rows written before it
func TestSecretsFallBackToPlaintextWithoutACipher(t *testing.T) {
	svc, repo := newTestService(t, Definition{Path: "shop/general/api_key", Type: TypeSecret})
	ctx := context.Background()

	if _, err := svc.Set(ctx, "shop/general/api_key", "dev_key", DefaultScope); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if stored := repo.rows[scopeKey(DefaultScope, "shop/general/api_key")].Value; stored != "dev_key" {
		t.Errorf("stored = %q, want plaintext when no cipher is configured", stored)
	}
	// switching the key on afterwards still reads the unencrypted row
	svc.SetCipher(testCipher(t))
	if got, err := svc.Get(ctx, "shop/general/api_key", DefaultScope); err != nil || got.Value != "dev_key" {
		t.Errorf("Get of a pre-cipher row = (%q, %v), want the plaintext back", got.Value, err)
	}
}

// A rotated-away or missing key is a misconfiguration, and reading garbage would be worse than failing
func TestGetFailsLoudlyWhenTheCipherIsGone(t *testing.T) {
	svc, repo := newTestService(t, Definition{Path: "shop/general/api_key", Type: TypeSecret})
	svc.SetCipher(testCipher(t))
	ctx := context.Background()
	if _, err := svc.Set(ctx, "shop/general/api_key", "sk_live_x", DefaultScope); err != nil {
		t.Fatalf("Set: %v", err)
	}

	stranded := NewService(repo) // same rows, no key
	if _, err := stranded.Get(ctx, "shop/general/api_key", DefaultScope); !errors.Is(err, ErrSecretCipherUnavailable) {
		t.Errorf("Get of an encrypted row without a cipher = %v, want ErrSecretCipherUnavailable", err)
	}
}

// An empty secret is a clear, not a value to encrypt - otherwise "unset" round-trips as ciphertext
func TestEmptySecretIsStoredAsEmpty(t *testing.T) {
	svc, repo := newTestService(t, Definition{Path: "shop/general/api_key", Type: TypeSecret})
	svc.SetCipher(testCipher(t))
	if _, err := svc.Set(context.Background(), "shop/general/api_key", "", DefaultScope); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if stored := repo.rows[scopeKey(DefaultScope, "shop/general/api_key")].Value; stored != "" {
		t.Errorf("stored = %q, want an empty string for a cleared secret", stored)
	}
}

// ListAll backs the admin config screen, so it must emit one row per registered field in declaration
// order regardless of what happens to be stored
func TestListAllCoversEveryDefinitionAndAppliesTheSameFallback(t *testing.T) {
	svc, _ := newTestService(t,
		Definition{Path: "shop/general/a", Type: TypeString, Default: "da"},
		Definition{Path: "shop/general/b", Type: TypeString, Default: "db"},
		Definition{Path: "shop/general/c", Type: TypeString, Default: "dc"},
	)
	ctx := context.Background()
	channelID := uuid.New()
	channel := Scope{Kind: ScopeChannel, ChannelID: &channelID}

	if _, err := svc.Set(ctx, "shop/general/b", "stored-default", DefaultScope); err != nil {
		t.Fatalf("Set b: %v", err)
	}
	if _, err := svc.Set(ctx, "shop/general/c", "channel-value", channel); err != nil {
		t.Fatalf("Set c: %v", err)
	}

	entries, err := svc.ListAll(ctx, channel)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("ListAll returned %d entries, want one per definition", len(entries))
	}
	for i, want := range []struct {
		path      string
		value     string
		isDefault bool
		scope     Scope
	}{
		{"shop/general/a", "da", true, DefaultScope},
		{"shop/general/b", "stored-default", false, DefaultScope},
		{"shop/general/c", "channel-value", false, channel},
	} {
		got := entries[i]
		if got.Definition.Path != want.path || got.Value != want.value || got.IsDefault != want.isDefault || got.Scope != want.scope {
			t.Errorf("entry %d = {%s %q default=%v scope=%s}, want {%s %q default=%v scope=%s}",
				i, got.Definition.Path, got.Value, got.IsDefault, FormatScope(got.Scope),
				want.path, want.value, want.isDefault, FormatScope(want.scope))
		}
	}
}

// publicConfig is unauthenticated, so anything not explicitly marked Public must never appear
func TestListPublicOnlyExposesPublicFields(t *testing.T) {
	svc, _ := newTestService(t,
		Definition{Path: "shop/general/name", Type: TypeString, Default: "Skelet", Public: true},
		Definition{Path: "shop/general/api_key", Type: TypeSecret, Default: "sk_live_x"},
		Definition{Path: "shop/general/internal", Type: TypeString, Default: "ops-only"},
	)
	entries, err := svc.ListPublic(context.Background(), DefaultScope)
	if err != nil {
		t.Fatalf("ListPublic: %v", err)
	}
	if len(entries) != 1 || entries[0].Definition.Path != "shop/general/name" {
		t.Fatalf("ListPublic = %v, want only the public field", entries)
	}
	for _, e := range entries {
		if e.Definition.Type == TypeSecret {
			t.Errorf("a secret-typed field reached the public list: %s", e.Definition.Path)
		}
	}
}

// Rows whose path was deleted from the registry are invisible to every read, so they need their own
// listing before anything can clean them up
func TestListOrphansAndPruneTargetOnlyUnregisteredPaths(t *testing.T) {
	svc, repo := newTestService(t, Definition{Path: "shop/general/name", Type: TypeString})
	ctx := context.Background()
	if _, err := svc.Set(ctx, "shop/general/name", "kept", DefaultScope); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// a row left behind by a field that no longer exists in code
	if _, err := repo.Upsert(ctx, DefaultScope, "shop/general/removed", "stale"); err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	orphans, err := svc.ListOrphans(ctx)
	if err != nil {
		t.Fatalf("ListOrphans: %v", err)
	}
	if len(orphans) != 1 || orphans[0].Path != "shop/general/removed" {
		t.Fatalf("ListOrphans = %v, want only the unregistered path", orphans)
	}

	n, err := svc.Prune(ctx)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Errorf("Prune = %d, want 1", n)
	}
	if _, still := repo.rows[scopeKey(DefaultScope, "shop/general/removed")]; still {
		t.Error("the orphan row survived Prune")
	}
	if _, gone := repo.rows[scopeKey(DefaultScope, "shop/general/name")]; !gone {
		t.Error("Prune deleted a registered path's row")
	}
}

// Repository failures have to surface rather than read as "nothing configured", which would quietly
// swap every live setting for its code default
func TestRepositoryErrorsPropagate(t *testing.T) {
	svc, repo := newTestService(t, Definition{Path: "shop/general/name", Type: TypeString, Default: "Skelet"})
	ctx := context.Background()
	boom := errors.New("connection refused")

	repo.listErr = boom
	if _, err := svc.Get(ctx, "shop/general/name", DefaultScope); !errors.Is(err, boom) {
		t.Errorf("Get with a failing repo = %v, want %v", err, boom)
	}
	if _, err := svc.ListAll(ctx, DefaultScope); !errors.Is(err, boom) {
		t.Errorf("ListAll with a failing repo = %v, want %v", err, boom)
	}
	if _, err := svc.ListOrphans(ctx); !errors.Is(err, boom) {
		t.Errorf("ListOrphans with a failing repo = %v, want %v", err, boom)
	}
	repo.listErr = nil

	repo.upsertErr = boom
	if _, err := svc.Set(ctx, "shop/general/name", "x", DefaultScope); !errors.Is(err, boom) {
		t.Errorf("Set with a failing repo = %v, want %v", err, boom)
	}
	repo.upsertErr = nil

	repo.deleteErr = boom
	if _, err := svc.Reset(ctx, "shop/general/name", DefaultScope); !errors.Is(err, boom) {
		t.Errorf("Reset with a failing repo = %v, want %v", err, boom)
	}
}

func channelScope(t *testing.T) Scope {
	t.Helper()
	id := uuid.New()
	return Scope{Kind: ScopeChannel, ChannelID: &id}
}

// A channel row on a platform-wide knob is never consulted by Get, so writing one has to fail loudly
// instead of leaving a value in the table that looks effective in the admin UI
func TestSetRefusesAScopeNothingReads(t *testing.T) {
	svc, repo := newTestService(t, Definition{Path: "shop/general/name", Type: TypeString, Scoping: ScopingGlobal})
	ctx := context.Background()

	if _, err := svc.Set(ctx, "shop/general/name", "Channel Name", channelScope(t)); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("Set at channel scope = %v, want ErrInvalidScope", err)
	}
	if _, err := svc.Reset(ctx, "shop/general/name", channelScope(t)); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("Reset at channel scope = %v, want ErrInvalidScope", err)
	}
	stored, _ := repo.ListAll(ctx)
	if len(stored) != 0 {
		t.Fatalf("refused writes still stored %d row(s)", len(stored))
	}
	if _, err := svc.Set(ctx, "shop/general/name", "Platform Name", DefaultScope); err != nil {
		t.Fatalf("Set at default scope: %v", err)
	}
}

// Settings have no per-locale dimension at all, so a locale in the scope is refused everywhere
func TestSetRefusesALocaleNothingReads(t *testing.T) {
	svc, _ := newTestService(t, Definition{Path: "shop/general/name", Type: TypeString, Scoping: ScopingChannel})
	ctx := context.Background()

	sc := channelScope(t)
	locale := "en-US"
	sc.Locale = &locale
	if _, err := svc.Set(ctx, "shop/general/name", "Localized", sc); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("Set at channel+locale scope = %v, want ErrInvalidScope", err)
	}
}

// publicConfig is what the storefront renders from, so it has to resolve at the caller's channel -
// reading it at the default scope served every channel the platform values
func TestListPublicResolvesTheCallersChannel(t *testing.T) {
	svc, _ := newTestService(t,
		Definition{Path: "shop/general/name", Type: TypeString, Default: "Skelet", Public: true},
	)
	ctx := context.Background()
	channel := channelScope(t)
	other := channelScope(t)

	if _, err := svc.Set(ctx, "shop/general/name", "Platform", DefaultScope); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if _, err := svc.Set(ctx, "shop/general/name", "Channel A", channel); err != nil {
		t.Fatalf("set channel: %v", err)
	}

	for _, c := range []struct {
		name  string
		scope Scope
		want  string
	}{
		{"the overriding channel", channel, "Channel A"},
		{"a channel without an override", other, "Platform"},
		{"the platform itself", DefaultScope, "Platform"},
	} {
		entries, err := svc.ListPublic(ctx, c.scope)
		if err != nil {
			t.Fatalf("ListPublic for %s: %v", c.name, err)
		}
		if len(entries) != 1 || entries[0].Value != c.want {
			t.Errorf("ListPublic for %s = %v, want %q", c.name, entries, c.want)
		}
	}
}

// Settings are never authored per locale, but a READ can still carry one (the admin header has a locale picker, and
// locale-scoped rows exist), and it has to resolve the channel's value rather than falling through to the platform default
func TestResolutionFallsBackFromLocaleToChannelBeforeDefault(t *testing.T) {
	svc, repo := newTestService(t,
		Definition{Path: "shop/general/name", Type: TypeString, Default: "Registry", Scoping: ScopingChannel},
	)
	ctx := context.Background()
	channel := channelScope(t)
	localized := channel
	locale := "de-DE"
	localized.Locale = &locale

	if _, err := svc.Set(ctx, "shop/general/name", "Platform", DefaultScope); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if _, err := svc.Set(ctx, "shop/general/name", "Channel", channel); err != nil {
		t.Fatalf("set channel: %v", err)
	}

	// No locale row yet: the channel value wins over the platform one, and says so
	got, err := svc.Get(ctx, "shop/general/name", localized)
	if err != nil {
		t.Fatalf("Get at channel+locale: %v", err)
	}
	if got.Value != "Channel" {
		t.Errorf("Get at channel+locale = %q, want the channel value", got.Value)
	}
	if got.Scope.Locale != nil || got.Scope.Kind != ScopeChannel {
		t.Errorf("Get reported scope %s, want the channel-wide scope the value came from", FormatScope(got.Scope))
	}
	all, err := svc.ListAll(ctx, localized)
	if err != nil {
		t.Fatalf("ListAll at channel+locale: %v", err)
	}
	if len(all) != 1 || all[0].Value != "Channel" {
		t.Errorf("ListAll at channel+locale = %v, want the channel value", all)
	}

	// A hand-inserted locale row still wins for that locale, since the resolver reads the most
	// specific row it finds
	if _, err := repo.Upsert(ctx, localized, "shop/general/name", "Locale row"); err != nil {
		t.Fatalf("seed a locale row: %v", err)
	}
	got, err = svc.Get(ctx, "shop/general/name", localized)
	if err != nil {
		t.Fatalf("Get with a locale row: %v", err)
	}
	if got.Value != "Locale row" || got.Scope.Locale == nil {
		t.Errorf("Get = %q at %s, want the locale row", got.Value, FormatScope(got.Scope))
	}
	// but writing one is refused now
	if _, err := svc.Set(ctx, "shop/general/name", "Nope", localized); !errors.Is(err, ErrInvalidScope) {
		t.Errorf("Set at channel+locale = %v, want ErrInvalidScope", err)
	}
}
