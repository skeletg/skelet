package setup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/skeletg/skelet/apps/skelet/internal/gitx"
	"github.com/skeletg/skelet/apps/skelet/internal/profile"
)

// Everything setup decides before it writes a file
type answers struct {
	target    string
	signals   bool
	ai        bool
	search    string
	gateways  []string
	adminSlug bool
	apps      []string
	// the subset of apps that runs in containers; the rest run on the host
	dockerApps []string
	tools      []string
	// fromProfile is a selection replayed rather than chosen now, which leaves the working tree as it is
	fromProfile bool
	// prune is whether the working tree follows the selection at all, removeDeselected whether that may take apps out
	prune, removeDeselected bool
}

const (
	kTarget  = "SKELET_TARGET"
	kSignals = "SKELET_SIGNALS"
	kAI      = "SKELET_AI"
	kSearch  = "SKELET_SEARCH"
	kGates   = "SKELET_GATEWAYS"
	kSlug    = "SKELET_ADMIN_SLUG"
	kApps    = "SKELET_APPS_SELECTED"
	kDocker  = "SKELET_DOCKER_APPS"
	kTools   = "SKELET_TOOLS_SELECTED"
)

// The profile's keys, in the order setup writes them
var profileKeys = []string{kTarget, kSignals, kAI, kSearch, kGates, kSlug, kApps, kDocker, kTools}

// errRefused is an answer that stops setup before it writes anything; the reason is already printed
var errRefused = errors.New("refused")

type prompter struct {
	on  bool
	in  *bufio.Reader
	out io.Writer
}

func (p prompter) yesNo(q string, def bool) bool {
	if !p.on {
		return def
	}
	hint := " [y/N] "
	if def {
		hint = " [Y/n] "
	}
	fmt.Fprint(p.out, q+hint)
	switch p.line() {
	case "":
		return def
	case "y", "Y", "yes", "YES":
		return true
	}
	return false
}

func (p prompter) choice(q, def string, opts ...string) string {
	if !p.on {
		return def
	}
	fmt.Fprintf(p.out, "    %s\n", q)
	for i, o := range opts {
		mark := ""
		if o == def {
			mark = "  (default)"
		}
		fmt.Fprintf(p.out, "      %d) %s%s\n", i+1, o, mark)
	}
	fmt.Fprintf(p.out, "    pick [1-%d] ", len(opts))
	if n, err := strconv.Atoi(p.line()); err == nil && n >= 1 && n <= len(opts) {
		return opts[n-1]
	}
	return def
}

func (p prompter) line() string {
	s, _ := p.in.ReadString('\n')
	return strings.TrimSpace(s)
}

func answer(root string, c catalog, f flags, prof profile.Profile, profilePath string, ask prompter, out io.Writer) (answers, error) {
	a := answers{target: "dev", signals: true, ai: true, search: "opensearch", gateways: c.gateways}
	if err := defaults(&a, prof); err != nil {
		return a, err
	}

	switch {
	case f.apps.set:
		a.apps = f.apps.items
	case has(prof, kApps):
		a.apps, a.fromProfile = known(prof[kApps], c.apps(), out), true
		fmt.Fprintf(out, "==> App selection: %s (from %s; change with skelet setup --apps ...)\n", orNone(a.apps, "backend only"), profilePath)
	case ask.on:
		fmt.Fprintln(out, "==> Which apps should this clone run?")
		fmt.Fprintln(out, "    The Go backend (GraphQL APIs, workers) always runs. The")
		fmt.Fprintln(out, "    frontends are optional - skip whichever you plan to replace")
		fmt.Fprintln(out, "    with your own build.")
		if ask.yesNo("    Include the admin dashboard (React SPA on :5173)?", true) {
			a.apps = append(a.apps, "dashboard")
		}
		for _, n := range c.storefronts {
			if ask.yesNo(fmt.Sprintf("    Include storefront %q (Next.js)?", n), true) {
				a.apps = append(a.apps, n)
			}
		}
	default:
		a.apps = c.apps()
	}

	// Where they run is a separate question from whether you have them, and the default is the host
	switch {
	case f.dockerApps.set:
		a.dockerApps = f.dockerApps.items
	case len(a.apps) == 0:
	case has(prof, kDocker):
		a.dockerApps = split(prof[kDocker])
	default:
		// No answer on record, so the placement the clone already runs with stands rather than being emptied
		current := placement(root)
		if ask.on {
			fmt.Fprintln(out, "==> Where should the frontends run?")
			fmt.Fprintln(out, "    On the host the dev server and the gates share one node_modules, so")
			fmt.Fprintln(out, "    a lint run checks the same tree the dev server is serving. In Docker")
			fmt.Fprintln(out, "    they are two separate trees, but you need no Node installed.")
			if ask.yesNo("    Run them in Docker instead of on the host?", len(current) > 0) {
				current = a.apps
			} else {
				current = nil
			}
		}
		a.dockerApps = current
	}
	// Only apps you selected, so --docker-apps can't containerize one that isn't here
	a.dockerApps = slices.DeleteFunc(slices.Clone(a.dockerApps), func(app string) bool { return !slices.Contains(a.apps, app) })

	// A host app needs pnpm at its pinned version - refused before anything is written, since finding out at the
	// install step leaves a half-set-up clone and a frontend that won't start
	if problem := pnpmProblem(root, a.hostApps()); problem != "" {
		fmt.Fprintf(os.Stderr, "==> The frontends on the host need pnpm: %s\n", problem)
		if !ask.yesNo("    Run them in Docker instead, which needs no pnpm on the host?", ask.on) {
			fmt.Fprintf(os.Stderr, "    Install it and re-run, or re-run with --docker-apps %s\n", join(a.apps))
			return a, errRefused
		}
		a.dockerApps = a.apps
	}

	a.prune = !f.keepCode && !a.fromProfile
	a.removeDeselected = true
	if a.prune && !f.apps.set && !a.fromProfile && ask.on && len(excluded(a, c)) > 0 {
		a.removeDeselected = ask.yesNo("    Also remove the deselected apps' source from your working tree?", true)
	}

	if ask.on {
		shape(&a, c, ask, out)
	}

	switch {
	case f.tools.set:
		a.tools = f.tools.items
	default:
		a.tools = split(prof[kTools])
		if ask.on && a.target == "dev" {
			fmt.Fprintln(out, "==> Optional dev tools - the stack runs without any of them")
			var picks []string
			for _, t := range tools {
				if ask.yesNo("    "+toolHints[t]+"?", slices.Contains(a.tools, t)) {
					picks = append(picks, t)
				}
			}
			a.tools = picks
		}
	}
	return a, nil
}

// The profile's answers become this run's defaults, so re-running is not an interrogation
func defaults(a *answers, p profile.Profile) error {
	bad := func(k string, want string) error {
		return fmt.Errorf("the profile says %s=%s - it takes %s", k, p[k], want)
	}
	if v, ok := p.Get(kTarget); ok {
		if v != "dev" && v != "small" && v != "prod" {
			return bad(kTarget, "dev, small or prod")
		}
		a.target = v
	}
	for k, dst := range map[string]*bool{kSignals: &a.signals, kAI: &a.ai} {
		if v, ok := p.Get(k); ok {
			if v != "true" && v != "false" {
				return bad(k, "true or false")
			}
			*dst = v == "true"
		}
	}
	if v, ok := p.Get(kSearch); ok {
		if v != "opensearch" && v != "postgres" {
			return bad(kSearch, "opensearch or postgres")
		}
		a.search = v
	}
	if v, ok := p.Get(kGates); ok {
		a.gateways = split(v)
	}
	if v, ok := p.Get(kSlug); ok {
		if v != "0" && v != "1" {
			return bad(kSlug, "0 or 1")
		}
		a.adminSlug = v == "1"
	}
	return nil
}

func shape(a *answers, c catalog, ask prompter, out io.Writer) {
	// "Deployment target" alone reads as where this will eventually live, so people building a shop pick prod and
	// then find skelet up refuses to start - so each one says what it actually does
	fmt.Fprintln(out, "==> How will you run THIS clone?")
	fmt.Fprintln(out, "      dev    the local stack - hot reload and sidecars, started with skelet up")
	fmt.Fprintln(out, "      small  a one-container server deployment - writes config, starts nothing here")
	fmt.Fprintln(out, "      prod   the full split-tier server deployment - writes config, starts nothing here")
	fmt.Fprintln(out, "    Pick dev unless you are configuring a server right now.")
	a.target = ask.choice("Target", a.target, "dev", "small", "prod")

	fmt.Fprintln(out, "==> Which suites do you want? (all default on)")
	a.signals = ask.yesNo("    Signals - analytics: Lens, Synapse, Pulse, Prism?", a.signals)
	a.ai = ask.yesNo("    AI - admin assistant, review classification?", a.ai)

	fmt.Fprintln(out, "==> Engines")
	a.search = ask.choice("Product search", a.search, "opensearch", "postgres")

	if len(c.gateways) == 0 {
		fmt.Fprintln(out, "==> Payment gateways - none in this tree (add one with `skelet cli plugin install`)")
		a.gateways = nil
	} else {
		fmt.Fprintln(out, "==> Payment gateways - unpicked ones are left out of the build entirely")
		var picks []string
		for _, g := range c.gateways {
			if ask.yesNo("    "+strings.TrimPrefix(g, "payment-")+"?", slices.Contains(a.gateways, g)) {
				picks = append(picks, g)
			}
		}
		a.gateways = picks
	}

	fmt.Fprintln(out, "==> Hardening")
	a.adminSlug = ask.yesNo("    Move the admin API off /api/admin/graphql to an unguessable path?", a.adminSlug)
}

func (a answers) hostApps() []string {
	var host []string
	for _, app := range a.apps {
		if !slices.Contains(a.dockerApps, app) {
			host = append(host, app)
		}
	}
	return host
}

// excluded is the sparse-checkout patterns that take the deselected apps out of the working tree
func excluded(a answers, c catalog) []string {
	var out []string
	for _, app := range c.apps() {
		if !slices.Contains(a.apps, app) {
			out = append(out, "!/"+appDir(app)+"/")
		}
	}
	return out
}

func has(p profile.Profile, k string) bool {
	_, ok := p.Get(k)
	return ok
}

// known keeps the apps this checkout has - a profile can outlive a storefront
func known(v string, apps []string, out io.Writer) []string {
	var keep []string
	for _, app := range split(v) {
		if slices.Contains(apps, app) {
			keep = append(keep, app)
		} else {
			fmt.Fprintf(out, "    (the profile names %q, which this checkout doesn't have - skipping it)\n", app)
		}
	}
	return keep
}

func placement(root string) []string {
	raw, _ := os.ReadFile(filepath.Join(root, ".skelet-apps"))
	return split(strings.Join(strings.Fields(string(raw)), ""))
}

func orNone(list []string, none string) string {
	if len(list) == 0 {
		return none
	}
	return join(list)
}

var pnpmPin = regexp.MustCompile(`"packageManager":\s*"pnpm@([0-9.]+)`)

// pnpmProblem says why pnpm can't run the host apps, or nothing when it can. It asks from inside the first app, where
// pnpm switches itself to the pinned version - that switch is what breaks
func pnpmProblem(root string, hostApps []string) string {
	if len(hostApps) == 0 {
		return ""
	}
	dir := appDir(hostApps[0])
	manifest, err := os.ReadFile(filepath.Join(root, dir, "package.json"))
	onDisk := err == nil
	if !onDisk {
		// a deselected app comes back from sparse-checkout only later, so its pin comes from HEAD
		manifest, _ = gitx.Show(root, dir+"/package.json")
	}
	want := ""
	if m := pnpmPin.FindSubmatch(manifest); m != nil {
		want = string(m[1])
	}
	if _, err := exec.LookPath("pnpm"); err != nil {
		return "no pnpm on PATH - corepack enable, or npm install -g pnpm@" + orNone(split(want), "<version>")
	}
	if want == "" || !onDisk {
		return ""
	}
	cmd := exec.Command("pnpm", "--version")
	cmd.Dir = filepath.Join(root, dir)
	raw, _ := cmd.Output()
	have := strings.TrimSpace(string(raw))
	if have != want {
		return fmt.Sprintf("pnpm in %s answers %s, the apps pin %s - pnpm self-update %s", dir, orNone(split(have), "nothing"), want, want)
	}
	return ""
}
