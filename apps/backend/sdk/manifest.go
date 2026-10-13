package sdk

// The host refuses to boot on a duplicate slug, an SDKRange excluding its own Version, a missing or out-of-range dependency, or a cycle It runs after Install replays the queue, so init() panics surface first - this is the second line of defence, catching what init() can't see, like a plugin built against SDK 1 loaded into a host running SDK 2 Slug is forever once shipped: it namespaces config (plugin/<slug>/*), the enabled flag and the migrations table, so renaming makes downgrade impossible without manual SQL

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// The number SDKRange gates against, kept honest by api_surface_test.go: api.json records the exported surface and apidiff re-checks it against the last release tag
const Version = "0.1.1" // 0.x while the surface is unfrozen - breaking changes ride the MINOR, additions the patch; 0.1.0 is the first release baseline, tagged apps/backend/sdk/v0.1.0 when the SDK is published

// A compile-time Go literal rather than a file the host reads at boot - no YAML parsing, no filesystem access, no "where does the manifest live" debate
// Generate any YAML mirror FROM this struct, never the reverse
type Manifest struct {
	Slug         string       // lowercase, dash-separated ASCII - unique across the host
	Name         string       // free-form label for dashboards
	Version      string       // this plugin's own semver - the host never gates on it, but other plugins can via Dependencies
	SDKRange     string       // range of host SDKs this works with, usually "^MAJOR.MINOR.PATCH"
	Dependencies []Dependency // checked for presence and version at boot, and used to order migration apply

	// Event types published, by convention "<slug>.<verb>_<object>" - the validator resolves every plugin's Consumes against these plus core's registry, so "A renamed its event and B's subscriber broke" surfaces at boot instead of at runtime
	// PublishAsync refuses a type missing from this list, so the declaration and the runtime can't drift
	// Boot refuses a name core already emits or another plugin declares - a publish under it would reach every subscriber and outbound webhook as that sender
	Emits []string
	// Every entry must resolve to a core event or another plugin's Emits or boot fails - the validator can't rewrite a subscriber's name, so a core rename only stays safe while both names are registered
	Consumes []string

	// What an UNREADABLE kill switch means for this plugin - fail_open (the zero value) keeps it serving through a config-store hiccup, fail_closed refuses instead Declared here rather than in config on purpose: during the outage this exists to survive, a config-stored policy is unreadable too, so it would always collapse back to fail_open It's also the plugin author's call, not the operator's - only the plugin knows whether "couldn't check" is safe, the way a fraud screen's isn't
	FailurePolicy FailurePolicy

	// Declared side effects, so an operator can read a third-party plugin's claimed scope before installing This is DISCLOSURE, not a sandbox, and it can never become one: a Tier 1 plugin is compiled into the process with full privileges, so no list constrains what it actually does - enforcing it would mean moving plugins out-of-process, which is a different tier, not a hardening pass Treat a claim here the way you'd treat a README, and audit the code for anything you don't trust Vocabulary: db.read, db.write, valkey.read, valkey.write, http.outbound:<host> (or :*), filesystem.read:<path>, filesystem.write:<path>, events.publish:<type>, events.consume:<type>, long-running (goroutines outliving a request)
	Capabilities []string

	// Browser-side origins this plugin's integration needs in the Content-Security-Policy, declared rather than registered because they are static facts about the plugin
	// cmd/secheadgen composes them into the header constants and the edge configs, so installing a gateway never means hand-editing core's security headers
	SecurityOrigins *SecurityOrigins
}

// Origins per CSP directive, e.g. Stripe declares js.stripe.com under Script and Frame, api.stripe.com under
// Connect - PaymentAllow feeds the Permissions-Policy payment=() allowlist for gateways using the Payment Request API
type SecurityOrigins struct {
	Script       []string
	Connect      []string
	Frame        []string
	FormAction   []string
	PaymentAllow []string
}

type FailurePolicy string

const (
	FailOpen   FailurePolicy = "" // zero value - an unreadable kill switch means "keep serving"
	FailClosed FailurePolicy = "fail_closed"
)

// Version is a range in the same dialect as SDKRange, matched against the dep's own Manifest.Version
type Dependency struct {
	Slug    string
	Version string
}

// Reads the auto-registered plugin/<slug>/enabled, defaulting true - uuid.Nil for default scope, a channel id for the per-channel override Instrument already gates payment hooks on this (and on the rate limit) and Subscribe* gates event handlers, so call it yourself only for code the host doesn't wrap - a goroutine you spawned, or a branch inside a scheduled job No host or no manifest returns true; an unreadable flag is what Manifest.FailurePolicy decides
func IsEnabled(ctx context.Context, slug string, channelID uuid.UUID) bool {
	h := hostNow()
	if h == nil {
		return true
	}
	return h.IsPluginEnabled(ctx, slug, channelID)
}

// Handle-bound, so a plugin can't accidentally check someone else's kill switch
func (p *Plugin) IsEnabled(ctx context.Context, channelID uuid.UUID) bool {
	return IsEnabled(ctx, p.slug, channelID)
}

// Carries the slug so nothing is inferred from call order - inferring it would attribute a registration to "whichever manifest registered most recently", which silently gave a plugin another's identity (wrong kill switch, wrong metrics, wrong scheduled-job row key) whenever init() ran in an unlucky order, and Go guarantees no order across sibling blank-imports
// Registrations keyed by their own path instead - permissions, config fields, events, migrations, data patches, dataloaders - stay package-level
type Plugin struct {
	slug string
}

func (p *Plugin) Slug() string { return p.slug }

// Call from init() - it works after Install too, since the host falls back to direct registration Panics on a duplicate slug or an obviously-bad manifest, which is what a misconfigured plugin deserves at boot - see the package doc for the full shape Same rule cmd/cli's scaffolder applies, so a hand-written manifest cannot produce a slug the tooling refuses
var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)

func RegisterPlugin(m Manifest) *Plugin {
	if err := validateManifestStatic(m); err != nil {
		panic(fmt.Sprintf("sdk.RegisterPlugin(%q): %v", m.Slug, err))
	}
	recordManifest(m)
	withHost(func(h Host) error { return h.RegisterPlugin(m) })
	return &Plugin{slug: m.Slug}
}

// Manifests are also kept package-side (not just replayed into the host) so build-time tooling like
// cmd/secheadgen can blank-import the plugin set and read declarations without standing up a host
var (
	manifestMu        sync.Mutex
	recordedManifests []Manifest
)

func recordManifest(m Manifest) {
	manifestMu.Lock()
	defer manifestMu.Unlock()
	recordedManifests = append(recordedManifests, m)
}

// RegisteredManifests returns every manifest RegisterPlugin has seen, in registration order
func RegisteredManifests() []Manifest {
	manifestMu.Lock()
	defer manifestMu.Unlock()
	out := make([]Manifest, len(recordedManifests))
	copy(out, recordedManifests)
	return out
}

// The checks that need no registry - empty fields, unparseable versions, malformed ranges
func validateManifestStatic(m Manifest) error {
	if strings.TrimSpace(m.Slug) == "" {
		return fmt.Errorf("manifest: empty slug")
	}
	// The slug is interpolated into route patterns and config paths, and `bin/cli plugin new` already refuses anything
	// else - registration accepted it, so a hand-written manifest could produce a broken namespace with no clear error
	if !slugPattern.MatchString(m.Slug) {
		return fmt.Errorf("manifest: slug %q must be lowercase letters, digits and dashes, starting with a letter (2-40 chars)", m.Slug)
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("manifest: empty version")
	}
	if _, err := parseSemver(m.Version); err != nil {
		return fmt.Errorf("manifest: bad version %q: %w", m.Version, err)
	}
	if strings.TrimSpace(m.SDKRange) == "" {
		return fmt.Errorf("manifest: empty SDKRange")
	}
	if _, err := parseRange(m.SDKRange); err != nil {
		return fmt.Errorf("manifest: bad SDKRange %q: %w", m.SDKRange, err)
	}
	// A typo here compiles, since the type is a string, and would quietly mean fail-open
	switch m.FailurePolicy {
	case FailOpen, FailClosed:
	default:
		return fmt.Errorf("manifest: FailurePolicy %q must be sdk.FailOpen or sdk.FailClosed", m.FailurePolicy)
	}
	for i, dep := range m.Dependencies {
		if strings.TrimSpace(dep.Slug) == "" {
			return fmt.Errorf("manifest: dep[%d] empty slug", i)
		}
		if strings.TrimSpace(dep.Version) == "" {
			return fmt.Errorf("manifest: dep[%d] (%s) empty version range", i, dep.Slug)
		}
		if _, err := parseRange(dep.Version); err != nil {
			return fmt.Errorf("manifest: dep[%d] (%s) bad version range %q: %w", i, dep.Slug, dep.Version, err)
		}
	}
	return nil
}

// Purpose-built parser for MAJOR.MINOR.PATCH plus one range expression - exact, ^ (same major), ~ (same major+minor), >= - with no pre-release or build metadata
// Masterminds/semver would be nicer but is a dependency for ~30 lines

type semver struct {
	Major, Minor, Patch int
}

func parseSemver(s string) (semver, error) {
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("expected MAJOR.MINOR.PATCH")
	}
	var nums [3]int
	for i, name := range [3]string{"major", "minor", "patch"} {
		n, err := semverPart(parts[i])
		if err != nil {
			return semver{}, fmt.Errorf("bad %s %q", name, parts[i])
		}
		nums[i] = n
	}
	return semver{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil
}

// Digits only, no leading zero - strconv.Atoi alone admits "+1" and "01", which semver does not
func semverPart(p string) (int, error) {
	if p == "" || (len(p) > 1 && p[0] == '0') || strings.TrimLeft(p, "0123456789") != "" {
		return 0, fmt.Errorf("not a semver number")
	}
	return strconv.Atoi(p)
}

func (a semver) compare(b semver) int {
	switch {
	case a.Major != b.Major:
		if a.Major < b.Major {
			return -1
		}
		return 1
	case a.Minor != b.Minor:
		if a.Minor < b.Minor {
			return -1
		}
		return 1
	case a.Patch != b.Patch:
		if a.Patch < b.Patch {
			return -1
		}
		return 1
	}
	return 0
}

type semverRange struct {
	floor semver
	op    rangeOp
}

type rangeOp int

const (
	opCaret rangeOp = iota // ^x.y.z: >=x.y.z, <(x+1).0.0
	opTilde                // ~x.y.z: >=x.y.z, <x.(y+1).0
	opExact                // x.y.z exact
	opGTE                  // >=x.y.z (no upper)
)

func parseRange(s string) (semverRange, error) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "^"):
		v, err := parseSemver(strings.TrimPrefix(s, "^"))
		if err != nil {
			return semverRange{}, err
		}
		return semverRange{floor: v, op: opCaret}, nil
	case strings.HasPrefix(s, "~"):
		v, err := parseSemver(strings.TrimPrefix(s, "~"))
		if err != nil {
			return semverRange{}, err
		}
		return semverRange{floor: v, op: opTilde}, nil
	case strings.HasPrefix(s, ">="):
		v, err := parseSemver(strings.TrimSpace(strings.TrimPrefix(s, ">=")))
		if err != nil {
			return semverRange{}, err
		}
		return semverRange{floor: v, op: opGTE}, nil
	default:
		v, err := parseSemver(s)
		if err != nil {
			return semverRange{}, err
		}
		return semverRange{floor: v, op: opExact}, nil
	}
}

func (r semverRange) satisfies(v semver) bool {
	if v.compare(r.floor) < 0 {
		return false
	}
	switch r.op {
	case opExact:
		return v.compare(r.floor) == 0
	case opTilde:
		return v.Major == r.floor.Major && v.Minor == r.floor.Minor
	case opCaret:
		// On 0.x the minor carries breaking changes, so caret pins it - ^1.2.0 admits 1.9.0, ^0.2.0 does not admit 0.9.0
		if r.floor.Major == 0 {
			return v.Major == 0 && v.Minor == r.floor.Minor
		}
		return v.Major == r.floor.Major
	case opGTE:
		return true
	}
	return false
}

func matchVersion(version, rng string) (bool, error) {
	v, err := parseSemver(version)
	if err != nil {
		return false, fmt.Errorf("version %q: %w", version, err)
	}
	r, err := parseRange(rng)
	if err != nil {
		return false, fmt.Errorf("range %q: %w", rng, err)
	}
	return r.satisfies(v), nil
}

// Lives in the SDK so the rules are expressed once and testable without booting a host - internal/plugin.ValidateManifests calls it hostSDKVersion is passed rather than read from the constant so tests can simulate a future SDK meeting older plugins knownEvents is core's OWN event set, plugin-registered names excluded; every Consumes must resolve to it or to a plugin's Emits, failing loud so a subscriber can't silently fall through, and no Emits may claim one of its names
func ValidateManifestSet(hostSDKVersion string, manifests []Manifest, bySlug map[string]Manifest, knownEvents map[string]struct{}) error {
	for _, m := range manifests {
		ok, err := matchVersion(hostSDKVersion, m.SDKRange)
		if err != nil {
			return fmt.Errorf("plugin %q: invalid SDKRange: %w", m.Slug, err)
		}
		if !ok {
			return fmt.Errorf("plugin %q requires SDK %s but host is %s", m.Slug, m.SDKRange, hostSDKVersion)
		}
		for _, dep := range m.Dependencies {
			depManifest, exists := bySlug[dep.Slug]
			if !exists {
				return fmt.Errorf("plugin %q depends on %q which is not registered", m.Slug, dep.Slug)
			}
			ok, err := matchVersion(depManifest.Version, dep.Version)
			if err != nil {
				return fmt.Errorf("plugin %q dep %q: invalid version range: %w", m.Slug, dep.Slug, err)
			}
			if !ok {
				return fmt.Errorf("plugin %q requires %q %s but installed version is %s", m.Slug, dep.Slug, dep.Version, depManifest.Version)
			}
		}
	}
	if err := detectCycle(manifests, bySlug); err != nil {
		return err
	}

	// Each name has one sender - a plugin publishing core's order.placed, or a second plugin's event, would hand every subscriber a payload from someone else
	available := make(map[string]struct{}, len(knownEvents))
	for k := range knownEvents {
		available[k] = struct{}{}
	}
	emitter := map[string]string{}
	for _, m := range manifests {
		for _, e := range m.Emits {
			if _, core := knownEvents[e]; core {
				return fmt.Errorf("plugin %q emits %q, which is a core event - publish under a name of its own", m.Slug, e)
			}
			if prev, taken := emitter[e]; taken && prev != m.Slug {
				return fmt.Errorf("plugins %q and %q both emit %q", prev, m.Slug, e)
			}
			emitter[e] = m.Slug
			available[e] = struct{}{}
		}
	}
	for _, m := range manifests {
		for _, c := range m.Consumes {
			if _, ok := available[c]; !ok {
				return fmt.Errorf("plugin %q consumes event %q but no plugin or core declares it (check plugin Emits + sdk.RegisterEvent)", m.Slug, c)
			}
		}
	}
	return nil
}

// Unregistered dep slugs need no handling here - the presence check above already failed for them
func detectCycle(manifests []Manifest, bySlug map[string]Manifest) error {
	const (
		unseen = 0
		onPath = 1
		done   = 2
	)
	state := make(map[string]int, len(manifests))
	var path []string

	var visit func(slug string) error
	visit = func(slug string) error {
		switch state[slug] {
		case onPath:
			start := 0
			for i, s := range path {
				if s == slug {
					start = i
					break
				}
			}
			cycle := append(append([]string{}, path[start:]...), slug)
			return fmt.Errorf("plugin dependency cycle: %s", strings.Join(cycle, " -> "))
		case done:
			return nil
		}
		state[slug] = onPath
		path = append(path, slug)
		m, ok := bySlug[slug]
		if ok {
			for _, dep := range m.Dependencies {
				if err := visit(dep.Slug); err != nil {
					return err
				}
			}
		}
		path = path[:len(path)-1]
		state[slug] = done
		return nil
	}
	for _, m := range manifests {
		if state[m.Slug] == unseen {
			if err := visit(m.Slug); err != nil {
				return err
			}
		}
	}
	return nil
}

// Every plugin lands after its transitive deps, and unrelated plugins keep registration order
// A cycle falls back to registration order rather than returning nil - ValidateManifestSet should have rejected it long before this runs
func TopoSortManifests(manifests []Manifest) []string {
	bySlug := make(map[string]Manifest, len(manifests))
	for _, m := range manifests {
		bySlug[m.Slug] = m
	}
	const (
		unseen = 0
		onPath = 1
		done   = 2
	)
	state := make(map[string]int, len(manifests))
	cycle := false
	out := make([]string, 0, len(manifests))

	var visit func(slug string)
	visit = func(slug string) {
		switch state[slug] {
		case onPath:
			cycle = true
			return
		case done:
			return
		}
		m, known := bySlug[slug]
		state[slug] = onPath
		if known {
			for _, dep := range m.Dependencies {
				visit(dep.Slug)
				if cycle {
					return
				}
			}
		}
		state[slug] = done
		// an unregistered dep is still walked so ordering around it stays sane, but emitting it would hand callers a name matching no plugin
		if known {
			out = append(out, slug)
		}
	}
	for _, m := range manifests {
		visit(m.Slug)
		if cycle {
			fallback := make([]string, len(manifests))
			for i, mm := range manifests {
				fallback[i] = mm.Slug
			}
			return fallback
		}
	}
	return out
}
