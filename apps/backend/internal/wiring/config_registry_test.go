package wiring

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// Importing wiring pulls in every domain package, so these assert over the WHOLE live config registry - config.Validate already runs at boot; the rules below are the ones nothing enforces

type emptyRepo struct{ stored map[string]string } // emptyRepo is a config.Repository with nothing stored, so every read falls through to the registry default

func (r *emptyRepo) Upsert(_ context.Context, scope config.Scope, path, value string) (config.StoredValue, error) {
	if r.stored != nil {
		r.stored[path] = value
	}
	return config.StoredValue{Scope: scope, Path: path, Value: value}, nil
}

func (r *emptyRepo) Delete(context.Context, config.Scope, string) error { return nil }

func (r *emptyRepo) GetCandidates(context.Context, config.Scope, string) ([]config.StoredValue, error) {
	return nil, nil
}

func (r *emptyRepo) ListInScopes(context.Context, config.Scope) ([]config.StoredValue, error) {
	return nil, nil
}

func (r *emptyRepo) ListAll(context.Context) ([]config.StoredValue, error) { return nil, nil }

func liveDefinitions(t *testing.T) []config.Definition {
	t.Helper()
	defs := config.AllDefinitions()
	// a registry this small means the domain init()s didn't run and every assertion below would vacuously pass
	if len(defs) < 50 {
		t.Fatalf("only %d config definitions registered, want the whole app's set - the imports that trigger init() are missing", len(defs))
	}
	return defs
}

func TestLiveConfigRegistryValidates(t *testing.T) { // the same check bootstrapRegistries runs at boot, hoisted into CI so a bad path fails the build not the deploy
	if err := config.Validate(); err != nil {
		t.Fatalf("config.Validate over the live registry: %v", err)
	}
}

func TestEveryConfigDefaultParsesAsItsDeclaredType(t *testing.T) { // a default the type rejects only fails when something first reads it, which can be a hot request months later
	svc := config.NewService(&emptyRepo{stored: map[string]string{}})
	ctx := context.Background()

	for _, d := range liveDefinitions(t) {
		// Set runs the real parseAndValidate on the value, which is exactly the gate an admin edit hits
		if _, err := svc.Set(ctx, d.Path, d.Default, config.DefaultScope); err != nil {
			if errors.Is(err, config.ErrInvalidValue) {
				t.Errorf("%s (type %s) declares default %q, which the type's own validator rejects", d.Path, d.Type, d.Default)
				continue
			}
			t.Errorf("%s: setting its own default = %v", d.Path, err)
		}
	}
}

func TestEveryConfigDefaultReadsBackThroughItsTypedAccessor(t *testing.T) { // an unset setting yields its default, so it has to come back usable through the accessor its Type implies
	svc := config.NewService(&emptyRepo{})
	ctx := context.Background()

	for _, d := range liveDefinitions(t) {
		switch d.Type {
		case config.TypeInt:
			if _, err := svc.GetInt(ctx, d.Path, config.DefaultScope); err != nil {
				t.Errorf("%s (int) default %q does not read back as an int: %v", d.Path, d.Default, err)
			}
		case config.TypeBool:
			if _, err := svc.GetBool(ctx, d.Path, config.DefaultScope); err != nil {
				t.Errorf("%s (bool) default %q does not read back as a bool: %v", d.Path, d.Default, err)
			}
		case config.TypeJSON, config.TypeWorkingHours, config.TypeSearchAttributes, config.TypeSortOptions:
			var out any
			if err := svc.GetJSON(ctx, d.Path, config.DefaultScope, &out); err != nil {
				t.Errorf("%s (%s) default %q does not read back as JSON: %v", d.Path, d.Type, d.Default, err)
			}
		}
	}
}

func TestNoSecretConfigFieldIsPublic(t *testing.T) { // Public means the unauthenticated publicConfig query returns it, so a public secret is a leaked credential
	for _, d := range liveDefinitions(t) {
		if d.Type == config.TypeSecret && d.Public {
			t.Errorf("%s is TypeSecret AND Public, so the unauthenticated publicConfig query would return the credential", d.Path)
		}
	}
}

func TestPublicConfigFieldsAreNotCredentialShaped(t *testing.T) { // the Type is the real gate, but a public path named like a credential is worth a second look before it ships
	suspicious := []string{"secret", "password", "private_key", "api_key", "token"}
	for _, d := range liveDefinitions(t) {
		if !d.Public {
			continue
		}
		lower := strings.ToLower(d.Path)
		for _, word := range suspicious {
			if strings.Contains(lower, word) {
				t.Errorf("%s is Public and its path contains %q - if it really is a credential it must be TypeSecret and private", d.Path, word)
			}
		}
	}
}

func TestEverySelectConfigFieldHasChoices(t *testing.T) { // neither options nor a provider renders a dropdown the admin cannot pick from
	for _, d := range liveDefinitions(t) {
		if d.Type != config.TypeSelect && d.Type != config.TypeMultiselect {
			continue
		}
		if len(d.Options) == 0 && d.Source == "" {
			t.Errorf("%s is %s with neither Options nor a Source, so its dropdown renders empty", d.Path, d.Type)
		}
	}
}

func TestEveryStaticSelectDefaultIsOneOfItsOptions(t *testing.T) { // a default off the list opens the form on a value it cannot re-select, so any save silently changes the setting
	for _, d := range liveDefinitions(t) {
		if d.Type != config.TypeSelect || len(d.Options) == 0 || d.Source != "" {
			continue
		}
		if d.Default == "" {
			continue // an empty default is "nothing picked yet", which the UI renders as a placeholder
		}
		var found bool
		for _, o := range d.Options {
			if o.Value == d.Default {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s defaults to %q, which is not among its options %v", d.Path, d.Default, optionValues(d.Options))
		}
	}
}

func optionValues(opts []config.Option) []string {
	out := make([]string, 0, len(opts))
	for _, o := range opts {
		out = append(out, o.Value)
	}
	return out
}

func TestEveryConfigFieldHasALabel(t *testing.T) { // every field renders a row in the admin UI, and a blank label renders a blank row
	for _, d := range liveDefinitions(t) {
		if strings.TrimSpace(d.Label) == "" {
			t.Errorf("%s has no Label, so the admin UI renders an unnamed row", d.Path)
		}
	}
}

func TestEveryConfigSectionAndGroupHasALabel(t *testing.T) { // sections and groups are the tabs and fieldsets, so the same rule applies
	for _, s := range config.AllSections() {
		if strings.TrimSpace(s.Label) == "" {
			t.Errorf("section %q has no Label", s.Slug)
		}
	}
	for _, g := range config.AllGroups() {
		if strings.TrimSpace(g.Label) == "" {
			t.Errorf("group %q/%q has no Label", g.Section, g.Slug)
		}
	}
}

func TestNoConfigPathIsPerLocale(t *testing.T) { // no knob is read per-locale, asserted across every path so a new setting cannot quietly opt in
	for _, d := range liveDefinitions(t) {
		if config.ScopingAllows(d.Path, config.ScopeChannel, true) {
			t.Errorf("%s accepts a locale-scoped write, but no knob is read per-locale", d.Path)
		}
		if config.ScopingAllows(d.Path, config.ScopeDefault, true) {
			t.Errorf("%s accepts a locale on the default scope", d.Path)
		}
	}
}

func TestChannelScopedWritesMatchChannelScopedReads(t *testing.T) { // a channel write that is never read means an operator sets a per-store value that never applies
	for _, d := range liveDefinitions(t) {
		allowsChannel := config.ScopingAllows(d.Path, config.ScopeChannel, false)
		isChannelScoped := config.ScopingFor(d.Path) == config.ScopingChannel
		if allowsChannel != isChannelScoped {
			t.Errorf("%s: ScopingAllows(channel) = %v but ScopingFor = %q - a write that is never read",
				d.Path, allowsChannel, config.ScopingFor(d.Path))
		}
		// the default scope is always readable, whatever the knob's scoping
		if !config.ScopingAllows(d.Path, config.ScopeDefault, false) {
			t.Errorf("%s refuses a default-scope write, which every knob must accept", d.Path)
		}
	}
}

func TestScopingAllowsRefusesUnknownKinds(t *testing.T) { // an unknown scope kind must be refused rather than treated as the default
	defs := liveDefinitions(t)
	path := defs[0].Path
	for _, kind := range []string{"", "website", "store", "DEFAULT"} {
		if config.ScopingAllows(path, kind, false) {
			t.Errorf("ScopingAllows(%q, %q) = true, want an unknown scope kind refused", path, kind)
		}
	}
}
