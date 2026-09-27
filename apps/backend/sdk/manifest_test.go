package sdk

import (
	"strings"
	"testing"
)

func TestParseSemver(t *testing.T) {
	cases := []struct {
		in   string
		want semver
		err  bool
	}{
		{"1.2.3", semver{1, 2, 3}, false},
		{"0.0.0", semver{0, 0, 0}, false},
		{"10.20.30", semver{10, 20, 30}, false},
		{"1.2", semver{}, true},
		{"1", semver{}, true},
		{"v1.2.3", semver{}, true},
		{"1.2.3-rc1", semver{}, true}, // pre-release not supported
		{"a.b.c", semver{}, true},
	}
	for _, c := range cases {
		got, err := parseSemver(c.in)
		if (err != nil) != c.err {
			t.Errorf("parseSemver(%q): err=%v, want err=%v", c.in, err, c.err)
			continue
		}
		if !c.err && got != c.want {
			t.Errorf("parseSemver(%q): got %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestRangeSatisfies(t *testing.T) {
	cases := []struct {
		rng     string
		version string
		want    bool
	}{
		// caret: same major, >= floor
		{"^1.0.0", "1.0.0", true},
		{"^1.0.0", "1.5.7", true},
		{"^1.0.0", "2.0.0", false},
		{"^1.2.3", "1.2.2", false}, // below floor
		{"^1.2.3", "1.2.3", true},
		{"^1.2.3", "1.9.0", true},

		// caret on 0.x pins the minor - that is where breaking changes land pre-1.0
		{"^0.5.0", "0.5.0", true},
		{"^0.5.0", "0.5.9", true},
		{"^0.5.0", "0.6.0", false},
		{"^0.5.0", "0.4.9", false},
		{"^0.5.0", "1.0.0", false},

		// tilde: same major+minor, >= floor
		{"~1.2.0", "1.2.0", true},
		{"~1.2.0", "1.2.99", true},
		{"~1.2.0", "1.3.0", false},

		// exact
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{"1.2.3", "1.2.2", false},

		// gte
		{">=1.0.0", "1.0.0", true},
		{">=1.0.0", "5.0.0", true},
		{">=1.0.0", "0.9.9", false},
	}
	for _, c := range cases {
		ok, err := matchVersion(c.version, c.rng)
		if err != nil {
			t.Errorf("matchVersion(%q, %q): unexpected err %v", c.version, c.rng, err)
			continue
		}
		if ok != c.want {
			t.Errorf("matchVersion(%q, %q): got %v, want %v", c.version, c.rng, ok, c.want)
		}
	}
}

func TestValidateManifestSet_SDKMismatch(t *testing.T) {
	manifests := []Manifest{
		{Slug: "p1", Version: "1.0.0", SDKRange: "^2.0.0"},
	}
	bySlug := map[string]Manifest{"p1": manifests[0]}
	err := ValidateManifestSet("1.5.0", manifests, bySlug, nil)
	if err == nil {
		t.Fatal("expected SDK incompatibility error")
	}
	if !strings.Contains(err.Error(), "SDK") {
		t.Errorf("error didn't mention SDK: %v", err)
	}
}

func TestValidateManifestSet_MissingDep(t *testing.T) {
	manifests := []Manifest{
		{Slug: "a", Version: "1.0.0", SDKRange: "^1.0.0", Dependencies: []Dependency{{Slug: "ghost", Version: "^1.0.0"}}},
	}
	bySlug := map[string]Manifest{"a": manifests[0]}
	err := ValidateManifestSet("1.0.0", manifests, bySlug, nil)
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected missing-dep error mentioning ghost; got %v", err)
	}
}

func TestValidateManifestSet_DepVersionMismatch(t *testing.T) {
	manifests := []Manifest{
		{Slug: "a", Version: "1.0.0", SDKRange: "^1.0.0", Dependencies: []Dependency{{Slug: "b", Version: "^2.0.0"}}},
		{Slug: "b", Version: "1.0.0", SDKRange: "^1.0.0"},
	}
	bySlug := map[string]Manifest{
		"a": manifests[0],
		"b": manifests[1],
	}
	err := ValidateManifestSet("1.0.0", manifests, bySlug, nil)
	if err == nil || !strings.Contains(err.Error(), "requires") {
		t.Fatalf("expected dep version mismatch; got %v", err)
	}
}

func TestValidateManifestSet_Cycle(t *testing.T) {
	manifests := []Manifest{
		{Slug: "a", Version: "1.0.0", SDKRange: "^1.0.0", Dependencies: []Dependency{{Slug: "b", Version: "^1.0.0"}}},
		{Slug: "b", Version: "1.0.0", SDKRange: "^1.0.0", Dependencies: []Dependency{{Slug: "c", Version: "^1.0.0"}}},
		{Slug: "c", Version: "1.0.0", SDKRange: "^1.0.0", Dependencies: []Dependency{{Slug: "a", Version: "^1.0.0"}}},
	}
	bySlug := map[string]Manifest{
		"a": manifests[0],
		"b": manifests[1],
		"c": manifests[2],
	}
	err := ValidateManifestSet("1.0.0", manifests, bySlug, nil)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle detection; got %v", err)
	}
}

func TestValidateManifestSet_OK(t *testing.T) {
	manifests := []Manifest{
		{Slug: "a", Version: "1.0.0", SDKRange: "^1.0.0", Dependencies: []Dependency{{Slug: "b", Version: "^1.0.0"}}},
		{Slug: "b", Version: "1.0.0", SDKRange: "^1.0.0"},
	}
	bySlug := map[string]Manifest{
		"a": manifests[0],
		"b": manifests[1],
	}
	if err := ValidateManifestSet("1.0.0", manifests, bySlug, nil); err != nil {
		t.Errorf("expected ok, got %v", err)
	}
}

func TestValidateManifestSet_ConsumesAndEmits(t *testing.T) {
	// B emits, A consumes - fine
	manifests := []Manifest{
		{Slug: "a", Version: "1.0.0", SDKRange: "^1.0.0", Consumes: []string{"b.thing_happened"}},
		{Slug: "b", Version: "1.0.0", SDKRange: "^1.0.0", Emits: []string{"b.thing_happened"}},
	}
	bySlug := map[string]Manifest{"a": manifests[0], "b": manifests[1]}
	if err := ValidateManifestSet("1.0.0", manifests, bySlug, nil); err != nil {
		t.Errorf("OK case rejected: %v", err)
	}

	// A consumes a core event from knownEvents - fine
	manifests = []Manifest{
		{Slug: "a", Version: "1.0.0", SDKRange: "^1.0.0", Consumes: []string{"core.order_placed"}},
	}
	bySlug = map[string]Manifest{"a": manifests[0]}
	known := map[string]struct{}{"core.order_placed": {}}
	if err := ValidateManifestSet("1.0.0", manifests, bySlug, known); err != nil {
		t.Errorf("core-event case rejected: %v", err)
	}

	// nobody emits it, so boot must reject
	manifests = []Manifest{
		{Slug: "a", Version: "1.0.0", SDKRange: "^1.0.0", Consumes: []string{"phantom.event"}},
	}
	bySlug = map[string]Manifest{"a": manifests[0]}
	err := ValidateManifestSet("1.0.0", manifests, bySlug, nil)
	if err == nil || !strings.Contains(err.Error(), "phantom.event") {
		t.Fatalf("expected dangling-consumer error mentioning phantom.event; got %v", err)
	}
}

func TestTopoSortManifests(t *testing.T) {
	// b depends on c, a on b, so dep order is c, b, a
	manifests := []Manifest{
		{Slug: "a", Version: "1.0.0", SDKRange: "^1.0.0", Dependencies: []Dependency{{Slug: "b", Version: "^1.0.0"}}},
		{Slug: "b", Version: "1.0.0", SDKRange: "^1.0.0", Dependencies: []Dependency{{Slug: "c", Version: "^1.0.0"}}},
		{Slug: "c", Version: "1.0.0", SDKRange: "^1.0.0"},
	}
	got := TopoSortManifests(manifests)
	want := []string{"c", "b", "a"}
	if len(got) != len(want) {
		t.Fatalf("len: got %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("topo[%d]: got %q, want %q (full: %v)", i, g, want[i], got)
		}
	}
}

// Callers treat each entry as a real plugin, so a phantom slug names nothing - ValidateManifestSet rejects this graph anyway, TopoSort just shouldn't invent entries on unvalidated input
func TestTopoSortManifests_SkipsUnregisteredDeps(t *testing.T) {
	manifests := []Manifest{
		{Slug: "a", Version: "1.0.0", SDKRange: "^1.0.0", Dependencies: []Dependency{{Slug: "ghost", Version: "^1.0.0"}}},
		{Slug: "b", Version: "1.0.0", SDKRange: "^1.0.0", Dependencies: []Dependency{{Slug: "a", Version: "^1.0.0"}}},
	}
	got := TopoSortManifests(manifests)
	for _, slug := range got {
		if slug == "ghost" {
			t.Fatalf("unregistered dep leaked into the sort output: %v", got)
		}
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want exactly the two registered plugins", got)
	}
	// skipping the phantom must not disturb the real edges
	if got[0] != "a" || got[1] != "b" {
		t.Errorf("dependency order lost: got %v, want [a b]", got)
	}
}

func TestRegisterPlugin_QueuesAndReplays(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	// pre-Install, so it queues
	RegisterPlugin(Manifest{Slug: "p1", Version: "1.0.0", SDKRange: "^1.0.0"})

	host := &fakeHost{}
	if len(host.pluginManifests) != 0 {
		t.Fatal("manifest landed before Install")
	}
	Install(host)
	if len(host.pluginManifests) != 1 {
		t.Fatalf("after Install: got %d, want 1", len(host.pluginManifests))
	}
	if host.pluginManifests[0].Slug != "p1" {
		t.Errorf("slug: got %q, want p1", host.pluginManifests[0].Slug)
	}
}

func TestRegisterPlugin_PanicsOnBadManifest(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on empty slug")
		}
	}()
	RegisterPlugin(Manifest{Version: "1.0.0", SDKRange: "^1.0.0"}) // empty slug
}

// The slug becomes part of a route pattern and a config path, so a malformed one produces a broken namespace
// rather than an error - cmd/cli's scaffolder always refused these, registration did not
func TestRegisterPluginRefusesMalformedSlugs(t *testing.T) {
	for _, slug := range []string{"Payment_Stripe", "payment stripe", "1payment", "payment/../admin", "a"} {
		t.Run(slug, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("slug %q was accepted", slug)
				}
			}()
			RegisterPlugin(Manifest{Slug: slug, Version: "1.0.0", SDKRange: "^1.0.0"})
		})
	}
}

func TestSemverRefusesSignsAndLeadingZeros(t *testing.T) {
	for _, in := range []string{"+1.2.3", "1.02.3", "01.2.3", "1.2.-3", "1..3"} {
		if _, err := parseSemver(in); err == nil {
			t.Errorf("parseSemver(%q) accepted a malformed version", in)
		}
	}
	if _, err := parseSemver("10.0.0"); err != nil {
		t.Errorf("parseSemver(10.0.0): %v", err)
	}
}

func TestAMisspelledFailurePolicyFailsBoot(t *testing.T) {
	m := Manifest{Slug: "fraud", Version: "0.1.0", SDKRange: "^0.2.0", FailurePolicy: "fail-closed"}
	if err := validateManifestStatic(m); err == nil {
		t.Fatal("a FailurePolicy typo was accepted - it would silently mean fail-open")
	}
	m.FailurePolicy = FailClosed
	if err := validateManifestStatic(m); err != nil {
		t.Fatalf("FailClosed: %v", err)
	}
}

func TestEachEventNameHasOneSender(t *testing.T) {
	core := map[string]struct{}{"order.placed": {}}
	for name, tc := range map[string]struct {
		manifests []Manifest
		want      string
	}{
		"a core name": {[]Manifest{{Slug: "a", Version: "0.1.0", SDKRange: "^0.2.0", Emits: []string{"order.placed"}}}, "core event"},
		"two plugins, one name": {[]Manifest{
			{Slug: "a", Version: "0.1.0", SDKRange: "^0.2.0", Emits: []string{"company.created"}},
			{Slug: "b", Version: "0.1.0", SDKRange: "^0.2.0", Emits: []string{"company.created"}},
		}, "both emit"},
		"its own names": {[]Manifest{{Slug: "a", Version: "0.1.0", SDKRange: "^0.2.0", Emits: []string{"company.created"}}}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			bySlug := map[string]Manifest{}
			for _, m := range tc.manifests {
				bySlug[m.Slug] = m
			}
			err := ValidateManifestSet("0.2.2", tc.manifests, bySlug, core)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("rejected a plugin's own event: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}
