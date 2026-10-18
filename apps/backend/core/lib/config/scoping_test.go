package config

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// The ladder is what lets a package annotate one group instead of every field in it, so the
// narrowest declaration has to win at each rung
func TestScopingForPrefersTheNarrowestDeclaration(t *testing.T) {
	isolateRegistries(t)
	RegisterSection(Section{Slug: "shop", Scoping: ScopingChannel})
	RegisterGroup(Group{Section: "shop", Slug: "inherits"})
	RegisterGroup(Group{Section: "shop", Slug: "pinned", Scoping: ScopingGlobal})
	RegisterSection(Section{Slug: "bare"})
	RegisterGroup(Group{Section: "bare", Slug: "general"})

	Register(Definition{Path: "shop/inherits/from_section", Type: TypeString})
	Register(Definition{Path: "shop/pinned/from_group", Type: TypeString})
	Register(Definition{Path: "shop/pinned/from_field", Type: TypeString, Scoping: ScopingChannel})
	Register(Definition{Path: "bare/general/undeclared", Type: TypeString})

	cases := map[string]Scoping{
		"shop/inherits/from_section": ScopingChannel,
		"shop/pinned/from_group":     ScopingGlobal,
		"shop/pinned/from_field":     ScopingChannel,
		"bare/general/undeclared":    ScopingGlobal, // nothing declared means platform-wide
		"nope/nope/nope":             ScopingGlobal, // unknown paths get the strictest answer
		"not-a-path":                 ScopingGlobal,
	}
	for path, want := range cases {
		if got := ScopingFor(path); got != want {
			t.Errorf("ScopingFor(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestScopingAllowsRejectsScopesNothingReads(t *testing.T) {
	isolateRegistries(t)
	RegisterSection(Section{Slug: "shop"})
	RegisterGroup(Group{Section: "shop", Slug: "general"})
	Register(Definition{Path: "shop/general/platform", Type: TypeString})
	Register(Definition{Path: "shop/general/per_channel", Type: TypeString, Scoping: ScopingChannel})

	cases := []struct {
		path      string
		kind      string
		hasLocale bool
		want      bool
	}{
		{"shop/general/platform", ScopeDefault, false, true},
		{"shop/general/platform", ScopeChannel, false, false},
		{"shop/general/per_channel", ScopeDefault, false, true},
		{"shop/general/per_channel", ScopeChannel, false, true},
		{"shop/general/per_channel", ScopeChannel, true, false}, // settings are never per-locale
		{"shop/general/platform", ScopeChannel, true, false},
	}
	for _, c := range cases {
		if got := ScopingAllows(c.path, c.kind, c.hasLocale); got != c.want {
			t.Errorf("ScopingAllows(%q, %q, locale=%v) = %v, want %v", c.path, c.kind, c.hasLocale, got, c.want)
		}
	}
}

// Every registration path validates its own value, so a typo fails at boot rather than silently
// downgrading a knob to platform-wide
func TestRegistrationRejectsUnknownScoping(t *testing.T) {
	isolateRegistries(t)
	mustPanic(t, "a section with a bogus scoping", func() {
		RegisterSection(Section{Slug: "bogus", Scoping: "per-channel"})
	})
	mustPanic(t, "a group with a bogus scoping", func() {
		RegisterGroup(Group{Section: "shop", Slug: "general", Scoping: "everywhere"})
	})
	mustPanic(t, "a field with a bogus scoping", func() {
		Register(Definition{Path: "shop/general/name", Type: TypeString, Scoping: "per-user"})
	})
	// "locale" was a level once - a leftover declaration must fail loudly, not silently widen a knob
	mustPanic(t, "the retired locale level", func() {
		Register(Definition{Path: "shop/general/localized", Type: TypeString, Scoping: "locale"})
	})
}

// Dynamic options are how a select offers database rows - customer groups, inventory locations - so an unregistered source has to surface rather than rendering an empty dropdown
func TestGetOptionsResolvesARegisteredSource(t *testing.T) {
	isolateRegistries(t)
	name := "test-source-" + uuid.NewString()[:8]
	RegisterSource(name, func(context.Context) ([]Option, error) {
		return []Option{{Value: "gold", Label: "Gold"}, {Value: "silver", Label: "Silver"}}, nil
	})

	got, err := Definition{Path: "a/b/c", Type: TypeSelect, Source: name}.GetOptions(context.Background())
	if err != nil {
		t.Fatalf("GetOptions: %v", err)
	}
	if len(got) != 2 || got[0].Value != "gold" || got[1].Value != "silver" {
		t.Errorf("GetOptions = %v, want the provider's two rows in order", got)
	}
}

func TestGetOptionsSurfacesAnUnknownSource(t *testing.T) {
	isolateRegistries(t)
	if _, err := (Definition{Path: "a/b/c", Type: TypeSelect, Source: "no-such-source"}).GetOptions(context.Background()); err == nil {
		t.Fatal("an unregistered source resolved, want the misconfiguration surfaced")
	}
}

// Swallowing a provider failure would render an empty dropdown that reads as "no choices exist" rather than "the lookup broke"
func TestGetOptionsPropagatesAProviderError(t *testing.T) {
	isolateRegistries(t)
	boom := errors.New("db down")
	name := "failing-source-" + uuid.NewString()[:8]
	RegisterSource(name, func(context.Context) ([]Option, error) { return nil, boom })

	if _, err := (Definition{Path: "a/b/c", Type: TypeSelect, Source: name}).GetOptions(context.Background()); !errors.Is(err, boom) {
		t.Errorf("GetOptions = %v, want the provider's error", err)
	}
}

// A static select carries its own options, so asking for them must not consult the provider registry
func TestGetOptionsReturnsStaticOptionsWithoutASource(t *testing.T) {
	isolateRegistries(t)
	got, err := (Definition{Path: "a/b/c", Type: TypeSelect, Options: []Option{{Value: "x", Label: "X"}}}).GetOptions(context.Background())
	if err != nil {
		t.Fatalf("GetOptions: %v", err)
	}
	if len(got) != 1 || got[0].Value != "x" {
		t.Errorf("GetOptions = %v, want the static option", got)
	}
	empty, err := (Definition{Path: "a/b/d", Type: TypeString}).GetOptions(context.Background())
	if err != nil {
		t.Fatalf("GetOptions on a plain field: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("GetOptions on a plain field = %v, want nothing", empty)
	}
}

// A nil cache is the "no Valkey wired" shape, and every read and write path has to tolerate it
func TestServiceWorksWithoutACache(t *testing.T) {
	svc, _ := newTestService(t, Definition{Path: "shop/general/name", Type: TypeString, Default: "Skelet", Label: "Name"})
	svc.SetCache(nil)
	ctx := context.Background()

	if _, err := svc.Get(ctx, "shop/general/name", DefaultScope); err != nil {
		t.Errorf("Get with no cache = %v, want nil", err)
	}
	if _, err := svc.Set(ctx, "shop/general/name", "Store", DefaultScope); err != nil {
		t.Errorf("Set with no cache = %v, want nil", err)
	}
	if _, err := svc.Reset(ctx, "shop/general/name", DefaultScope); err != nil {
		t.Errorf("Reset with no cache = %v, want nil", err)
	}
	if _, err := svc.ListPublic(ctx, DefaultScope); err != nil {
		t.Errorf("ListPublic with no cache = %v, want nil", err)
	}
}
