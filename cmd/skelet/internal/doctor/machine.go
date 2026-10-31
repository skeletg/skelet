package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

// The oldest git with sparse-checkout set --no-cone, which setup trims the working tree with
var minGit = []int{2, 35}

func docker(r *report) {
	r.section("docker")
	if exec.Command("docker", "info").Run() == nil {
		r.pass("docker daemon reachable")
	} else {
		r.bad("docker daemon not reachable - is Docker running?")
	}
	if v, err := output("", "docker", "compose", "version", "--short"); err == nil {
		r.pass("docker compose v2 present (%s)", v)
	} else {
		r.bad("docker compose v2 missing")
	}
	// Both modes are fine - containers run as you either way. Shown for debugging
	if opts, _ := output("", "docker", "info", "-f", "{{.SecurityOptions}}"); strings.Contains(opts, "rootless") {
		r.pass("rootless docker (containers run as uid 0 = you)")
	} else {
		r.pass("rootful docker (containers run as your uid via SKELET_UID)")
	}
}

// tools is what runs on the host itself: git always, and Go for lint and the codegen steps that don't need a container.
// root is empty outside a checkout, where there is no go.mod to hold Go to
func tools(r *report, root string) {
	r.section("tools")
	if out, err := output("", "git", "--version"); err != nil {
		r.bad("no git - skelet new fetches projects with it, and updates arrive through it")
	} else if v := version(out); older(v, minGit) {
		r.note("git %s is older than 2.35 - setup can't take deselected apps out of the working tree", dotted(v))
	} else {
		r.pass("git %s", dotted(v))
	}

	cli(r)

	want := goDirective(root)
	out, err := output("", "go", "version")
	if err != nil {
		if want != nil {
			r.note("no Go on the host - skelet lint and parts of skelet gen run it there (go.mod wants %s)", dotted(want))
		} else {
			r.note("no Go on the host - a project's skelet lint and parts of skelet gen run it there")
		}
		return
	}
	have := version(out)
	switch {
	case want == nil || !older(have, want):
		r.pass("go %s", dotted(have))
	case fetches():
		r.pass("go %s - fetches %s for this repo on its own", dotted(have), dotted(want))
	default:
		r.note("go %s is older than go.mod's %s, and GOTOOLCHAIN stops it fetching that one", dotted(have), dotted(want))
	}
}

// cli is the skelet on the PATH. A link to a checkout's ./skelet, the bash front door from before the CLI, worked
// as one - and stops working once a release takes that file away
func cli(r *report) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, "skelet")
		fi, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(p)
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, target)
			}
			if shim(filepath.Dir(target)) {
				r.note("skelet on the PATH links to %s, the ./skelet that goes away - install the CLI the README's way, with --force to replace the link", target)
				return
			}
		}
		r.pass("skelet %s", p)
		return
	}
	r.note("no skelet on the PATH - ./skelet goes away, so install the CLI the README's way")
}

// shim says dir is a checkout whose root skelet is the bash front door
func shim(dir string) bool {
	for _, marker := range []string{"bin/_lib", "cmd/skelet/go.mod"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func fetches() bool {
	v, _ := output("", "go", "env", "GOTOOLCHAIN")
	return strings.Contains(v, "auto")
}

func goDirective(root string) []int {
	if root == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(root, "apps", "backend", "core", "go.mod"))
	if err != nil {
		return nil
	}
	if m := regexp.MustCompile(`(?m)^go (\S+)`).FindSubmatch(raw); m != nil {
		return version(string(m[1]))
	}
	return nil
}

func disk(r *report, dir string) {
	r.section("disk")
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		r.note("can't read free space: %v", err)
		return
	}
	gb := uint64(st.Bavail) * uint64(st.Bsize) / (1 << 30)
	if gb >= 10 {
		r.pass("%dG free", gb)
	} else {
		r.note("only %dG free - docker system prune?", gb)
	}
}

var numbers = regexp.MustCompile(`\d+(\.\d+)*`)

// version is the first dotted number in s
func version(s string) []int {
	var v []int
	for p := range strings.SplitSeq(numbers.FindString(s), ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v
		}
		v = append(v, n)
	}
	return v
}

func older(v, than []int) bool { return slices.Compare(v, than) < 0 }

func dotted(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ".")
}

func output(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
