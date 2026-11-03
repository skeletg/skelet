// Package launch is how the installed skelet runs a project's own CLI once the project has no shim to: it builds
// cmd/skelet from the project's tree - once per change, cached - and execs it. Always from the project's source,
// never a downloaded release, since a project may change its CLI. The build flags, SKELET_ROOT and SKELET_LAUNCHED
// are a contract released binaries depend on, so they change only with abi
package launch

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const (
	abi = "1"
	// LDFlags makes the build a checkout one - without the -X, main.mode stays global
	LDFlags = "-s -w -X main.mode=checkout"
	// keep is how many builds the shared cache holds, besides each project's last good one
	keep = 8
)

// Run builds the project's CLI if it has to and becomes it
func Run(root string, args []string) int {
	bin, err := Build(root, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet:", err)
		return 1
	}
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "SKELET_ROOT=") || strings.HasPrefix(kv, "SKELET_LAUNCHED=") || strings.HasPrefix(kv, "SKELET_CLI=")
	})
	env = append(env, "SKELET_ROOT="+root, "SKELET_LAUNCHED="+abi)
	if self, err := os.Executable(); err == nil && !strings.Contains(self, string(filepath.Separator)+"go-build") {
		env = append(env, "SKELET_CLI="+self)
	}
	return sys.Replace(bin, append([]string{bin}, args...), env)
}

// Build returns the path of the project's CLI, compiling it first when its source changed since the last build
func Build(root string, log io.Writer) (string, error) {
	src := filepath.Join(root, "cmd", "skelet")
	goVersion, err := directive(src)
	if err != nil {
		return "", err
	}
	key, err := Key(src, goVersion)
	if err != nil {
		return "", err
	}
	dir := cacheDir(root)
	bin := filepath.Join(dir, "skelet-"+key)
	if usable(bin) {
		touch(bin)
		return bin, nil
	}
	unlock, err := lock(dir)
	if err != nil {
		return "", err
	}
	defer unlock()
	// Another run may have built it while this one waited for the lock
	if usable(bin) {
		touch(bin)
		return bin, nil
	}
	fmt.Fprintln(log, "==> building the skelet CLI for this project (once per change to cmd/skelet)")
	if err := compile(src, goVersion, bin); err != nil {
		if last := lastGood(dir, root); last != "" && os.Getenv("SKELET_STRICT") == "" {
			fmt.Fprintf(log, "%v\nskelet: cmd/skelet doesn't build - running the last build that did. SKELET_STRICT=1 makes this an error\n", err)
			return last, nil
		}
		return "", err
	}
	remember(dir, root, bin)
	prune(dir)
	return bin, nil
}

// Key names a build by everything that goes into it: the source minus tests and testdata - all of it, not just .go,
// so an embedded file counts - the Go version, the platform and the launcher's own contract
func Key(src, goVersion string) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "abi %s\ngo %s\n%s/%s\n", abi, goVersion, runtime.GOOS, runtime.GOARCH)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != src && (name == "testdata" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		if strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, ".") || rel == "skelet" || !d.Type().IsRegular() {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(raw))
		h.Write(raw)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

var goLine = regexp.MustCompile(`(?m)^go ([0-9][0-9.]*)`)

// directive is the Go version cmd/skelet's go.mod asks for, which also names the image a Docker build uses - so a
// release binary never builds a newer project with the image it shipped knowing about
func directive(src string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(src, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("this project has no CLI source at %s: %w", src, err)
	}
	m := goLine.FindSubmatch(raw)
	if m == nil {
		return "", fmt.Errorf("%s/go.mod has no go directive", src)
	}
	return string(m[1]), nil
}

// compile builds with host Go, and in the golang image only when there is no Go or it's too old to fetch the one
// go.mod asks for - a compile error isn't retried in Docker, where it would only print twice
func compile(src, goVersion, bin string) error {
	tmp := fmt.Sprintf("%s.tmp-%d", bin, os.Getpid())
	defer func() { _ = os.Remove(tmp) }()
	var err error
	if _, lookErr := exec.LookPath("go"); lookErr == nil {
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", LDFlags, "-o", tmp, ".")
		cmd.Dir = src
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-buildvcs=false", "CGO_ENABLED=0")
		out, buildErr := cmd.CombinedOutput()
		if buildErr != nil && !strings.Contains(string(out), "requires go >=") {
			return fmt.Errorf("building cmd/skelet failed:\n%s", strings.TrimSpace(string(out)))
		}
		err = buildErr
	} else {
		err = lookErr
	}
	if err != nil {
		if err := inDocker(src, goVersion, tmp); err != nil {
			return err
		}
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, bin)
}

// inDocker builds in the golang image and streams the binary out over stdout, so the container writes nothing into
// the checkout and its uid never matters
func inDocker(src, goVersion, out string) error {
	if exec.Command("docker", "info").Run() != nil {
		return errors.New("building the CLI needs Go on the host, or a running Docker - neither is here")
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	cmd := exec.Command("docker", "run", "--rm", "-v", src+":/src:ro", "-v", "skelet-cli-gocache:/root/.cache/go-build",
		"-w", "/src", "-e", "CGO_ENABLED=0", "-e", "GOOS="+runtime.GOOS, "-e", "GOARCH="+runtime.GOARCH,
		"-e", "GOFLAGS=-buildvcs=false", "golang:"+goVersion+"-alpine",
		"sh", "-c", "go build -trimpath -ldflags '"+LDFlags+"' -o /tmp/skelet . && cat /tmp/skelet")
	var stderr strings.Builder
	cmd.Stdout, cmd.Stderr = f, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building cmd/skelet in golang:%s-alpine failed:\n%s", goVersion, strings.TrimSpace(stderr.String()))
	}
	return f.Close()
}

// cacheDir is shared by every checkout and worktree and survives git clean; the checkout's own .skelet/bin is the
// fallback when there's no usable user cache
func cacheDir(root string) string {
	if base, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(base, "skelet", "bin")
		if os.MkdirAll(dir, 0o755) == nil {
			return dir
		}
	}
	dir := filepath.Join(root, ".skelet", "bin")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func lock(dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func usable(bin string) bool {
	fi, err := os.Stat(bin)
	return err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0
}

// touch marks a build used, so pruning drops the ones no project runs any more
func touch(bin string) {
	now := time.Now()
	_ = os.Chtimes(bin, now, now)
}

// The project's last good build, kept so a cmd/skelet that stops compiling doesn't take down every verb - doctor and
// the commit hook included
func marker(dir, root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(dir, "last-"+hex.EncodeToString(sum[:])[:12])
}

func remember(dir, root, bin string) { _ = os.WriteFile(marker(dir, root), []byte(bin), 0o644) }

func lastGood(dir, root string) string {
	raw, err := os.ReadFile(marker(dir, root))
	if err != nil || !usable(string(raw)) {
		return ""
	}
	return string(raw)
}

// prune keeps the most recently used builds, and every project's last good one whatever its age. Run under the lock
func prune(dir string) {
	protected := map[string]bool{}
	markers, _ := filepath.Glob(filepath.Join(dir, "last-*"))
	for _, m := range markers {
		if raw, err := os.ReadFile(m); err == nil {
			protected[string(raw)] = true
		}
	}
	builds, _ := filepath.Glob(filepath.Join(dir, "skelet-*"))
	builds = slices.DeleteFunc(builds, func(b string) bool { return strings.Contains(filepath.Base(b), ".tmp-") })
	mtime := func(p string) time.Time {
		fi, err := os.Stat(p)
		if err != nil {
			return time.Time{}
		}
		return fi.ModTime()
	}
	sort.Slice(builds, func(i, j int) bool { return mtime(builds[i]).After(mtime(builds[j])) })
	for i, b := range builds {
		if i >= keep && !protected[b] {
			_ = os.Remove(b)
		}
	}
}
