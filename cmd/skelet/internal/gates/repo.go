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
shows up for someone else, long after the commit that caused it: the installer is POSIX sh, no tracked file
names a bin/ script (they're all gone), every compose env_file has a template, the root .env.example covers
what prod compose demands, and the changelog is well-formed.`

// Files that name a retired script on purpose: the hooks package and the doctor recognise the hook the old installer
// script wrote, test fixtures hold that hook and an old profile's header as they were, and this gate's own test names
// a missing script to see it caught
var retiredOnPurpose = []string{
	"cmd/skelet/internal/hooks",
	"cmd/skelet/internal/doctor/checkout.go",
	"cmd/skelet/internal/doctor/doctor_test.go",
	"cmd/skelet/internal/profile/profile_test.go",
	"cmd/skelet/internal/gates/repo_test.go",
}

func Repo(root string, _ []string) int { return repoCheck(root, os.Stdout) }

func repoCheck(root string, out io.Writer) int {
	r := &report{out: out}
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

var binReference = regexp.MustCompile(`(^|[^A-Za-z0-9_./-])(bin/[a-z0-9][a-z0-9_-]*)`)

// references checks every tracked text file - released changelogs aside, which record what was true then - for a
// bin/ script, since every one became a skelet verb and a mention would send someone to a file that isn't there
func references(r *report, root string) {
	r.section("no tracked file names a bin/ script")
	args := []string{"grep", "-IhoE", `(^|[^A-Za-z0-9_./-])bin/[a-z0-9][a-z0-9_-]*`, "--", ".", ":!changelog/[0-9]*.md"}
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
