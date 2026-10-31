package gates

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/changelog"
)

const RepoAbout = `Structural invariants a fresh clone or a forked storefront leans on - the class of breakage that only
shows up for someone else, long after the commit that caused it: the scripts parse and run, the
installer is POSIX sh, every verb's script exists and every script has a verb, every script that
creates containers sources bin/_lib, no tracked file names a bin/ script that's gone, every compose
env_file has a template, the root .env.example covers what prod compose demands, and the changelog
is well-formed.`

const table = "cmd/skelet/internal/route/table.go"

// forwarders are scripts kept only to hand an old caller to the CLI, with no verb of their own - bin/check is what
// hooks written before skelet hooks run
var forwarders = []string{"check"}

// Files that name a retired script on purpose: the hooks package recognises the hook the old installer script wrote,
// test fixtures hold that hook and an old profile's header as they were, and this gate's own test names missing
// scripts to see them caught
var retiredOnPurpose = []string{
	"cmd/skelet/internal/hooks",
	"cmd/skelet/internal/doctor/doctor_test.go",
	"cmd/skelet/internal/profile/profile_test.go",
	"cmd/skelet/internal/gates/repo_test.go",
}

func Repo(root string, _ []string) int { return repoCheck(root, os.Stdout) }

func repoCheck(root string, out io.Writer) int {
	r := &report{out: out}
	scripts, _ := filepath.Glob(filepath.Join(root, "bin", "*"))
	slices.Sort(scripts)

	r.section("bin/* parse and are executable")
	before := r.failed
	n := 0
	for _, p := range append([]string{filepath.Join(root, "skelet")}, scripts...) {
		rel := "./skelet"
		if p != filepath.Join(root, "skelet") {
			rel, _ = filepath.Rel(root, p)
		}
		if msg, err := exec.Command("bash", "-n", p).CombinedOutput(); err != nil {
			fmt.Fprint(out, string(msg))
			r.bad("%s is not valid bash", rel)
		}
		// core.fileMode is false on WSL clones, so a lost +x bit is invisible to git diff and only surfaces when
		// someone else checks the file out
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm()&0o111 == 0 {
			r.bad("%s is not executable (git update-index --chmod=+x %s)", rel, rel)
		}
		n++
	}
	if r.failed == before {
		r.pass("%d scripts", n)
	}

	// It runs as curl ... | sh, and sh is dash on Debian and Ubuntu - so dash when there is one, since bash -n waves
	// bashisms through
	r.section("the CLI installer is POSIX sh")
	posix := "sh"
	if _, err := exec.LookPath("dash"); err == nil {
		posix = "dash"
	}
	if exec.Command(posix, "-n", filepath.Join(root, "cmd", "skelet", "install.sh")).Run() == nil {
		r.pass("cmd/skelet/install.sh parses with %s", posix)
	} else {
		r.bad("cmd/skelet/install.sh is not valid sh")
	}

	routes(r, root, scripts)
	sourcesLib(r, root, scripts)
	references(r, root)
	envTemplates(r, root)
	required(r, root)

	r.section("the changelog index, release files and fragments agree")
	var cl bytes.Buffer
	if changelog.Check(root, &cl) != 0 {
		r.failed = true
	}
	fmt.Fprint(out, cl.String())

	fmt.Fprintln(out)
	if r.failed {
		fmt.Fprintln(out, "==> repo-check FAILED")
	} else {
		fmt.Fprintln(out, "==> repo-check OK")
	}
	return r.code()
}

var (
	scriptRoute = regexp.MustCompile(`Script: "([a-z][a-z0-9-]*)"`)
	goRoute     = regexp.MustCompile(`Go: [a-z]+\.[A-Z]`)
)

// routes reads the route table as text, not a second list, so it can't drift - and a floor over every route, scripts
// and Go verbs alike, so a refactor that stops matching fails loudly instead of passing vacuously
func routes(r *report, root string, scripts []string) {
	r.section("./skelet and bin/* agree")
	src, _ := os.ReadFile(filepath.Join(root, table))
	var routed []string
	for _, m := range scriptRoute.FindAllSubmatch(src, -1) {
		if s := string(m[1]); !slices.Contains(routed, s) {
			routed = append(routed, s)
		}
	}
	slices.Sort(routed)
	if total := len(routed) + len(goRoute.FindAll(src, -1)); total < 30 {
		r.bad("only %d routes found in %s - the table or this check moved, not the scripts", total, table)
		return
	}
	missing := false
	for _, s := range routed {
		if fi, err := os.Stat(filepath.Join(root, "bin", s)); err != nil || fi.Mode().Perm()&0o111 == 0 {
			r.bad("the route table sends a verb to bin/%s, which is missing or not executable", s)
			missing = true
		}
	}
	if !missing {
		r.pass("%d routed scripts all present", len(routed))
	}
	// The other direction - a new script nobody wired into the front door is invisible to anyone reading skelet help,
	// the only list a newcomer sees
	unreachable := false
	for _, p := range scripts {
		b := filepath.Base(p)
		if strings.HasPrefix(b, "_") || slices.Contains(routed, b) || slices.Contains(forwarders, b) {
			continue
		}
		r.bad("bin/%s has no skelet verb - add a route in %s, or the front door is incomplete", b, table)
		unreachable = true
	}
	if !unreachable {
		r.pass("every bin script is reachable through a verb")
	}
}

var createsContainers = regexp.MustCompilePOSIX(`docker compose ([a-z-]+ )*(run|up)([^a-zA-Z0-9_]|$)`)

// sourcesLib - run and up are the verbs that create a container, and the uid it runs as comes from SKELET_UID, which
// only bin/_lib exports. exec and ps against one that already exists don't care, so it's scoped to those two
func sourcesLib(r *report, root string, scripts []string) {
	r.section("container-creating scripts source bin/_lib")
	missing := false
	for _, p := range scripts {
		if strings.HasPrefix(filepath.Base(p), "_") {
			continue
		}
		raw, _ := os.ReadFile(p)
		if !createsContainers.Match(raw) {
			continue
		}
		if !regexp.MustCompile(`(?m)^\. bin/_lib`).Match(raw) {
			rel, _ := filepath.Rel(root, p)
			r.bad("%s creates containers without sourcing bin/_lib - they would run as the wrong uid", rel)
			missing = true
		}
	}
	if !missing {
		r.pass("every one of them does")
	}
}

var binReference = regexp.MustCompile(`(^|[^A-Za-z0-9_./-])(bin/[a-z0-9][a-z0-9_-]*)`)

// references checks every tracked text file - released changelogs aside, which record what was true then - for a
// bin/ script that no longer exists. Each port deletes scripts, so this is what makes it reword their mentions too
func references(r *report, root string) {
	r.section("every referenced bin/* script exists")
	args := []string{"grep", "-IhoE", `(^|[^A-Za-z0-9_./-])bin/[a-z0-9][a-z0-9_-]*`, "--", ".",
		":!changelog/[0-9]*.md", ":!cmd/skelet/internal/route/testdata"}
	for _, f := range retiredOnPurpose {
		args = append(args, ":!"+f)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	hits, _ := cmd.Output()
	var seen []string
	missing := false
	for line := range strings.SplitSeq(string(hits), "\n") {
		m := binReference.FindStringSubmatch(line)
		if m == nil || slices.Contains(seen, m[2]) {
			continue
		}
		seen = append(seen, m[2])
		if _, err := os.Stat(filepath.Join(root, m[2])); err != nil {
			r.bad("referenced but missing: %s", m[2])
			missing = true
		}
	}
	if !missing {
		r.pass("no dangling references")
	}
}

// envTemplates - the git-ignored copy exists on your box either way; this is whether a fresh clone can boot every
// compose shape
func envTemplates(r *report, root string) {
	r.section("every compose env_file has a committed .example")
	composes, _ := filepath.Glob(filepath.Join(root, "docker-compose*.yml"))
	var files []string
	for _, c := range composes {
		raw, _ := os.ReadFile(c)
		for _, m := range regexp.MustCompile(`env/[a-z0-9.-]+\.env`).FindAll(raw, -1) {
			if f := string(m); !slices.Contains(files, f) {
				files = append(files, f)
			}
		}
	}
	slices.Sort(files)
	missing := false
	for _, f := range files {
		cmd := exec.Command("git", "ls-files", "--error-unmatch", f+".example")
		cmd.Dir = root
		if cmd.Run() != nil {
			r.bad("%s has no committed %s.example", f, f)
			missing = true
		}
	}
	if !missing {
		r.pass("every env_file has a template")
	}
}

// required - ${VAR:?} means compose refuses to start without it, and those come from the root .env, which is
// git-ignored, so the template beside it is the only thing telling a new deployer they exist
func required(r *report, root string) {
	r.section("the root .env.example covers what prod compose demands")
	var vars []string
	for _, c := range []string{"docker-compose.prod.yml", "docker-compose.small.yml"} {
		raw, _ := os.ReadFile(filepath.Join(root, c))
		for _, m := range regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*):\?`).FindAllSubmatch(raw, -1) {
			if v := string(m[1]); !slices.Contains(vars, v) {
				vars = append(vars, v)
			}
		}
	}
	slices.Sort(vars)
	example, _ := os.ReadFile(filepath.Join(root, ".env.example"))
	missing := false
	for _, v := range vars {
		if !regexp.MustCompile(`(?m)^[# \t]*` + v + `=`).Match(example) {
			r.bad("compose refuses to start without %s, and .env.example never mentions it", v)
			missing = true
		}
	}
	if !missing {
		r.pass("every required interpolation has a template line")
	}
}
