package config

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The three registries are package globals every domain package appends to from init(), so a test
// that registers anything hands the real contents back afterwards
func isolateRegistries(t *testing.T) {
	t.Helper()
	mu.Lock()
	savedSections, savedGroups, savedDefs := sections, groups, registry
	sections, groups, registry = nil, nil, nil
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		sections, groups, registry = savedSections, savedGroups, savedDefs
		mu.Unlock()
	})
}

func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic, want a registration panic", what)
		}
	}()
	fn()
}

func TestParsePathWantsExactlyThreeNonEmptySegments(t *testing.T) {
	section, group, field, ok := ParsePath("catalog/search/results_per_page")
	if !ok || section != "catalog" || group != "search" || field != "results_per_page" {
		t.Errorf("ParsePath = (%q, %q, %q, %v), want the three segments split out", section, group, field, ok)
	}

	for _, bad := range []string{
		"",
		"catalog",
		"catalog/search",
		"catalog/search/results/extra",
		"/search/field",
		"catalog//field",
		"catalog/search/",
	} {
		if _, _, _, ok := ParsePath(bad); ok {
			t.Errorf("ParsePath(%q) = ok, want it rejected", bad)
		}
	}
}

// Two packages claiming one slug means whichever ran second is invisible to Find - a boot panic beats
// an admin editing a field that writes somewhere else
func TestRegistrationPanicsOnDuplicates(t *testing.T) {
	isolateRegistries(t)
	RegisterSection(Section{Slug: "shop", Label: "Shop"})
	mustPanic(t, "a duplicate section", func() { RegisterSection(Section{Slug: "shop"}) })

	RegisterGroup(Group{Section: "shop", Slug: "general"})
	mustPanic(t, "a duplicate (section, group)", func() { RegisterGroup(Group{Section: "shop", Slug: "general"}) })
	// the same group slug under a different section is a different group
	RegisterGroup(Group{Section: "shop", Slug: "checkout"})

	Register(Definition{Path: "shop/general/name", Type: TypeString})
	mustPanic(t, "a duplicate path", func() { Register(Definition{Path: "shop/general/name", Type: TypeInt}) })
}

// Register deliberately accepts a path whose section and group don't exist yet, since init() order is
// undefined - Validate is the pass that catches it, and wiring runs it once everything has registered
func TestValidateCatchesStructuralErrorsRegisterCannot(t *testing.T) {
	isolateRegistries(t)
	RegisterSection(Section{Slug: "shop"})
	RegisterGroup(Group{Section: "shop", Slug: "general"})
	Register(Definition{Path: "shop/general/name", Type: TypeString})
	if err := Validate(); err != nil {
		t.Fatalf("a fully-referenced registry = %v, want nil", err)
	}

	Register(Definition{Path: "shop/ghost/name", Type: TypeString})
	Register(Definition{Path: "ghost/general/name", Type: TypeString})
	Register(Definition{Path: "not-a-path", Type: TypeString})

	err := Validate()
	if err == nil {
		t.Fatal("Validate = nil, want the three structural problems reported")
	}
	for _, want := range []string{"unknown group", "unknown section", "not section/group/field"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate error = %v, want it to mention %q", err, want)
		}
	}
}

// ListPublic filters on Public alone and secret masking lives in the admin resolver, so this pairing
// would hand a decrypted credential to anonymous callers - Validate has to refuse the boot
func TestValidateRejectsAPublicSecret(t *testing.T) {
	isolateRegistries(t)
	RegisterSection(Section{Slug: "shop"})
	RegisterGroup(Group{Section: "shop", Slug: "keys"})

	Register(Definition{Path: "shop/keys/api_key", Type: TypeSecret})
	Register(Definition{Path: "shop/keys/store_name", Type: TypeString, Public: true})
	if err := Validate(); err != nil {
		t.Fatalf("a private secret alongside a public string = %v, want nil", err)
	}

	Register(Definition{Path: "shop/keys/leaky", Type: TypeSecret, Public: true})
	err := Validate()
	if err == nil {
		t.Fatal("Validate = nil, want a public secret rejected")
	}
	for _, want := range []string{"shop/keys/leaky", "public and secret"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate error = %v, want it to mention %q", err, want)
		}
	}
}

// The admin UI renders fields in registration order, so All* must not reorder or alias the registry
func TestAccessorsReturnCopiesInRegistrationOrder(t *testing.T) {
	isolateRegistries(t)
	RegisterSection(Section{Slug: "shop"})
	RegisterGroup(Group{Section: "shop", Slug: "general"})
	Register(Definition{Path: "shop/general/b", Type: TypeString})
	Register(Definition{Path: "shop/general/a", Type: TypeString})

	defs := AllDefinitions()
	if len(defs) != 2 || defs[0].Path != "shop/general/b" || defs[1].Path != "shop/general/a" {
		t.Errorf("AllDefinitions = %v, want registration order preserved", pathsOf(defs))
	}
	defs[0].Default = "clobbered"
	if got, _ := Find("shop/general/b"); got.Default != "" {
		t.Error("editing the slice AllDefinitions returned reached the registry")
	}

	AllSections()[0].Label = "clobbered"
	if got, _ := findSectionLocked("shop"); got.Label != "" {
		t.Error("editing the slice AllSections returned reached the registry")
	}
	AllGroups()[0].Label = "clobbered"
	if got, _ := findGroupLocked("shop", "general"); got.Label != "" {
		t.Error("editing the slice AllGroups returned reached the registry")
	}
}

func pathsOf(defs []Definition) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Path)
	}
	return out
}

func TestFindersMissUnknownKeys(t *testing.T) {
	isolateRegistries(t)
	if _, ok := Find("nope/nope/nope"); ok {
		t.Error("Find on an unregistered path = true, want false")
	}
	if _, ok := findSectionLocked("nope"); ok {
		t.Error("an unregistered section slug = found, want not found")
	}
	if _, ok := findGroupLocked("nope", "nope"); ok {
		t.Error("an unregistered group pair = found, want not found")
	}
}

// Values are stored as strings, so normalizing at write time is what makes "007" and "7" compare
// equal later - without it every read has to re-parse to know whether anything changed
func TestParseAndValidateNormalizes(t *testing.T) {
	for name, tc := range map[string]struct {
		typ  Type
		in   string
		want string
	}{
		"int drops leading zeros":  {TypeInt, "007", "7"},
		"negative int":             {TypeInt, "-12", "-12"},
		"bool from 1":              {TypeBool, "1", "true"},
		"bool from TRUE":           {TypeBool, "TRUE", "true"},
		"bool from 0":              {TypeBool, "0", "false"},
		"json loses whitespace":    {TypeJSON, `{"a":  1,  "b": [2, 3]}`, `{"a":1,"b":[2,3]}`},
		"working hours is json":    {TypeWorkingHours, `[ {"day": 1} ]`, `[{"day":1}]`},
		"color lowercases":         {TypeColor, "#AABBCC", "#aabbcc"},
		"color trims":              {TypeColor, "  #aabbcc  ", "#aabbcc"},
		"empty color means unset":  {TypeColor, "   ", ""},
		"phone becomes e164":       {TypePhone, "+381 64 123 4567", "+381641234567"},
		"empty phone means unset":  {TypePhone, "  ", ""},
		"string passes through":    {TypeString, "  spaced  ", "  spaced  "},
		"secret passes through":    {TypeSecret, "sk_live_abc", "sk_live_abc"},
		"multiselect is untouched": {TypeMultiselect, "a,b", "a,b"},
	} {
		got, err := parseAndValidate(tc.typ, tc.in)
		if err != nil {
			t.Errorf("%s: parseAndValidate(%q) = %v, want %q", name, tc.in, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: parseAndValidate(%q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}

// A bad color reaches the storefront's inline styles and a bad phone its tel: links, so they're the basic types that validate
func TestParseAndValidateRejectsBadValues(t *testing.T) {
	for name, tc := range map[string]struct {
		typ Type
		in  string
	}{
		"int from words":        {TypeInt, "many"},
		"int from a float":      {TypeInt, "1.5"},
		"int from empty":        {TypeInt, ""},
		"bool from maybe":       {TypeBool, "maybe"},
		"json from a bare word": {TypeJSON, "{not json}"},
		"json from empty":       {TypeJSON, ""},
		"color without a hash":  {TypeColor, "aabbcc"},
		"color too short":       {TypeColor, "#abc"},
		"color with a bad char": {TypeColor, "#aabbcz"},
		"color with a script":   {TypeColor, "#aa;url(x)"},
		"phone without a +":     {TypePhone, "064 123 4567"},
		"phone from words":      {TypePhone, "call us"},
		"unknown type":          {Type("carrier-pigeon"), "x"},
	} {
		if got, err := parseAndValidate(tc.typ, tc.in); err == nil {
			t.Errorf("%s: parseAndValidate(%q) = %q, want an error", name, tc.in, got)
		}
	}
}

func TestIsHexColor(t *testing.T) {
	for _, ok := range []string{"#000000", "#ffffff", "#AABBCC", "#a1b2c3"} {
		if !isHexColor(ok) {
			t.Errorf("isHexColor(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "#", "#fff", "#fffffff", "ffffff", "#gggggg", "#ff ff0"} {
		if isHexColor(bad) {
			t.Errorf("isHexColor(%q) = true, want false", bad)
		}
	}
}

func TestValidateScope(t *testing.T) {
	id := uuid.New()
	locale := "en"
	blank := "  "

	for name, tc := range map[string]struct {
		scope   Scope
		wantErr bool
	}{
		"default":                     {Scope{Kind: ScopeDefault}, false},
		"default with a channel":      {Scope{Kind: ScopeDefault, ChannelID: &id}, true},
		"default with a locale":       {Scope{Kind: ScopeDefault, Locale: &locale}, true},
		"channel":                     {Scope{Kind: ScopeChannel, ChannelID: &id}, false},
		"channel and locale":          {Scope{Kind: ScopeChannel, ChannelID: &id, Locale: &locale}, false},
		"channel without an id":       {Scope{Kind: ScopeChannel}, true},
		"channel with a blank locale": {Scope{Kind: ScopeChannel, ChannelID: &id, Locale: &blank}, true},
		"unknown kind":                {Scope{Kind: "website"}, true},
		"empty kind":                  {Scope{}, true},
	} {
		if err := validateScope(tc.scope); (err != nil) != tc.wantErr {
			t.Errorf("%s: validateScope = %v, wantErr %v", name, err, tc.wantErr)
		}
	}
}

// scopeKey is the merge key in ListAll, so two different scopes must never collide on it
func TestScopeKeyDistinguishesEveryScope(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	en, de := "en", "de"
	keys := map[string]string{
		"default":      scopeKey(DefaultScope, "shop/general/name"),
		"channel a":    scopeKey(Scope{Kind: ScopeChannel, ChannelID: &a}, "shop/general/name"),
		"channel b":    scopeKey(Scope{Kind: ScopeChannel, ChannelID: &b}, "shop/general/name"),
		"channel a en": scopeKey(Scope{Kind: ScopeChannel, ChannelID: &a, Locale: &en}, "shop/general/name"),
		"channel a de": scopeKey(Scope{Kind: ScopeChannel, ChannelID: &a, Locale: &de}, "shop/general/name"),
		"other path":   scopeKey(DefaultScope, "shop/general/other"),
	}
	seen := make(map[string]string, len(keys))
	for name, key := range keys {
		if prev, dup := seen[key]; dup {
			t.Errorf("%s and %s both produce key %q", name, prev, key)
		}
		seen[key] = name
	}
}

func TestFormatScope(t *testing.T) {
	id := uuid.MustParse("018f0000-0000-7000-8000-000000000001")
	locale := "de"
	for name, tc := range map[string]struct {
		scope Scope
		want  string
	}{
		"default":            {DefaultScope, "default"},
		"channel":            {Scope{Kind: ScopeChannel, ChannelID: &id}, "channel:" + id.String()},
		"channel and locale": {Scope{Kind: ScopeChannel, ChannelID: &id, Locale: &locale}, "channel:" + id.String() + ":de"},
		"channel without id": {Scope{Kind: ScopeChannel}, "channel"},
		"unknown kind":       {Scope{Kind: "website"}, "website"},
	} {
		if got := FormatScope(tc.scope); got != tc.want {
			t.Errorf("%s: FormatScope = %q, want %q", name, got, tc.want)
		}
	}
}
