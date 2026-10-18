package config

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// The scalar types the config system can serialize and validate - JSON covers any structured value, stored as canonical whitespace-stripped JSON produced by json.Marshal after parse
type Type string

const (
	TypeString           Type = "string"
	TypeTextarea         Type = "textarea"
	TypeInt              Type = "int"
	TypeBool             Type = "bool"
	TypePhone            Type = "phone"
	TypeSelect           Type = "select"
	TypeMultiselect      Type = "multiselect"
	TypeWorkingHours     Type = "working_hours"
	TypeJSON             Type = "json"
	TypeSearchAttributes Type = "search_attributes"
	TypeSortOptions      Type = "sort_options"
	// A media reference stored as a string - either a plain URL or a JSON-serialized media-library AssetRef - The admin UI renders an upload, library picker or URL field, and the value is opaque to the backend
	TypeImage Type = "image"
	// A CSS hex color, "#rrggbb" lowercased, or "" when unset - validated on write, rendered as a swatch and hex picker
	TypeColor Type = "color"
	// A string stored encrypted at rest and never returned by the API, masked to "" - gateway keys and the like
	TypeSecret Type = "secret"
)

// Which scopes a knob's value is actually honoured at - the admin UI only offers these, and Set rejects anything deeper, so an override can never be stored somewhere nothing will read it Declare it on the Definition, or on its Group / Section to cover a whole family - the narrowest declaration wins, and an unset one means global, so per-channel support is opt-in Config is deliberately not a per-locale dimension: translated content lives on the entities that own it, not in settings, so the ladder stops at the channel
type Scoping string

const (
	// Platform-wide - read at DefaultScope, so a channel row would never be consulted
	ScopingGlobal Scoping = "global"
	// Per channel, falling back to the platform default
	ScopingChannel Scoping = "channel"
)

func validScoping(sc Scoping) bool {
	return sc == "" || sc == ScopingGlobal || sc == ScopingChannel
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Dynamic config options, e.g. the customer groups from the DB - register providers by name in init()
type SourceProvider func(ctx context.Context) ([]Option, error)

// A single config field's schema entry - registration order inside a group is the admin UI's rendering order
type Definition struct {
	Path    string
	Type    Type
	Default string // always the string serialization
	Label   string
	// What turning this knob actually does, one or two sentences - rendered as help text under the field and published in docs/settings.md
	// Optional: an empty one renders as the label alone, so descriptions can be backfilled a group at a time rather than all at once
	Description string
	Public      bool // true → returned by the unauthenticated publicConfig query
	Options     []Option
	Source      string  // name of a registered SourceProvider
	Scoping     Scoping // unset inherits the Group, then the Section, then global
	// Which settings store holds the value - unset is core's, and a Signals path is written and read
	// through the suite, whose processes are the ones reading it on their hot paths
	Store Store
}

type Store string

const (
	StoreCore    Store = ""
	StoreSignals Store = "signals"
)

// A top-level grouping - the admin UI renders one tab per section
type Section struct {
	Slug      string
	Label     string
	SortOrder int
	Scoping   Scoping // the default for every group and field under it
}

// A fieldset inside a section - (Section, Slug) is unique
type Group struct {
	Section   string
	Slug      string
	Label     string
	SortOrder int
	Scoping   Scoping // the default for every field in it, overriding the Section's
}

// Every path is exactly three slash-separated segments, section / group / field, which drives the admin UI layout - section is the left-nav tab, group the fieldset, field the input
// Paths that don't match are rejected by Register
const pathDepth = 3

// The three live registries, private so domain packages mutate them only through the Register* functions - readers iterate via the All*() accessors
var (
	mu        sync.RWMutex
	sections  []Section
	groups    []Group
	registry  []Definition
	providers = make(map[string]SourceProvider)
)

// Adds a named provider for dynamic options - call from init()
func RegisterSource(name string, p SourceProvider) {
	mu.Lock()
	defer mu.Unlock()
	providers[name] = p
}

func (d Definition) GetOptions(ctx context.Context) ([]Option, error) {
	if d.Source != "" {
		mu.RLock()
		p, ok := providers[d.Source]
		mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("unknown config source %q", d.Source)
		}
		return p(ctx)
	}
	return d.Options, nil
}

// Call from an init() in whatever package owns this area of the product - panics on a duplicate slug, since two packages registering the same section is a bug
func RegisterSection(s Section) {
	if !validScoping(s.Scoping) {
		panic(fmt.Sprintf("config: section %q has unknown scoping %q", s.Slug, s.Scoping))
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range sections {
		if existing.Slug == s.Slug {
			panic(fmt.Sprintf("config: section %q already registered", s.Slug))
		}
	}
	sections = append(sections, s)
}

// Panics on a duplicate (section, slug) pair
func RegisterGroup(g Group) {
	if !validScoping(g.Scoping) {
		panic(fmt.Sprintf("config: group %q/%q has unknown scoping %q", g.Section, g.Slug, g.Scoping))
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range groups {
		if existing.Section == g.Section && existing.Slug == g.Slug {
			panic(fmt.Sprintf("config: group %q/%q already registered", g.Section, g.Slug))
		}
	}
	groups = append(groups, g)
}

// Panics on a duplicate path - it does NOT check that the section and group references are known, so call Validate() once every package has registered to catch structural errors
func Register(d Definition) {
	if !validScoping(d.Scoping) {
		panic(fmt.Sprintf("config: path %q has unknown scoping %q", d.Path, d.Scoping))
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range registry {
		if existing.Path == d.Path {
			panic(fmt.Sprintf("config: path %q already registered", d.Path))
		}
	}
	registry = append(registry, d)
}

// Checks that every Definition's path is well-formed and references a known section and group - call once at startup after every init() has run, and it returns an aggregated message on any structural error
func Validate() error {
	mu.RLock()
	defer mu.RUnlock()
	var problems []string
	for _, d := range registry {
		section, group, _, ok := ParsePath(d.Path)
		if !ok {
			problems = append(problems, fmt.Sprintf("path %q is not section/group/field", d.Path))
			continue
		}
		if _, ok := findSectionLocked(section); !ok {
			problems = append(problems, fmt.Sprintf("path %q references unknown section %q", d.Path, section))
		}
		if _, ok := findGroupLocked(section, group); !ok {
			problems = append(problems, fmt.Sprintf("path %q references unknown group %q under section %q", d.Path, group, section))
		}
		// ListPublic filters on Public alone, and the masking that hides secrets lives in the admin resolver rather than under it - so this
		// pairing would serve a decrypted credential to anonymous callers. Refusing to boot beats discovering it in a public response
		if d.Public && d.Type == TypeSecret {
			problems = append(problems, fmt.Sprintf("path %q is both public and secret, which would expose its decrypted value to unauthenticated callers", d.Path))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("config registry invalid: %s", strings.Join(problems, "; "))
}

func AllSections() []Section {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Section, len(sections))
	copy(out, sections)
	return out
}

func AllGroups() []Group {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Group, len(groups))
	copy(out, groups)
	return out
}

// A copy of the registered fields in registration order, which is the order the admin UI renders in
func AllDefinitions() []Definition {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Definition, len(registry))
	copy(out, registry)
	return out
}

// The scoping in force for a path - the field's own, else its group's, else its section's, else global
// An unknown path answers global, the most restrictive answer
func ScopingFor(path string) Scoping {
	mu.RLock()
	defer mu.RUnlock()
	sectionSlug, groupSlug, _, ok := ParsePath(path)
	if !ok {
		return ScopingGlobal
	}
	for _, d := range registry {
		if d.Path == path && d.Scoping != "" {
			return d.Scoping
		}
	}
	if g, ok := findGroupLocked(sectionSlug, groupSlug); ok && g.Scoping != "" {
		return g.Scoping
	}
	if sec, ok := findSectionLocked(sectionSlug); ok && sec.Scoping != "" {
		return sec.Scoping
	}
	return ScopingGlobal
}

// Whether a knob's value is read at `scope` - a channel row on a global knob is never consulted, and
// no knob is per-locale, so writing either is refused rather than silently ignored
func ScopingAllows(path string, kind string, hasLocale bool) bool {
	if hasLocale {
		return false
	}
	switch kind {
	case ScopeDefault:
		return true
	case ScopeChannel:
		return ScopingFor(path) == ScopingChannel
	default:
		return false
	}
}

func Find(path string) (Definition, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, d := range registry {
		if d.Path == path {
			return d, true
		}
	}
	return Definition{}, false
}

// Splits a config path into its three segments - ok=false for anything without exactly three non-empty ones
func ParsePath(path string) (section, group, field string, ok bool) {
	parts := strings.Split(path, "/")
	if len(parts) != pathDepth {
		return "", "", "", false
	}
	if slices.Contains(parts, "") {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// The mutex-free bodies used by Validate() and the locked public finders - callers must hold mu
func findSectionLocked(slug string) (Section, bool) {
	for _, s := range sections {
		if s.Slug == slug {
			return s, true
		}
	}
	return Section{}, false
}

func findGroupLocked(sectionSlug, groupSlug string) (Group, bool) {
	for _, g := range groups {
		if g.Section == sectionSlug && g.Slug == groupSlug {
			return g, true
		}
	}
	return Group{}, false
}
