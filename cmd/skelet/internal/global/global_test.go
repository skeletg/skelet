package global

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/project"
)

// A test that reached a real exec would replace the test binary with the project's ./skelet - run from this repo,
// that's real commands against the developer's own stack. Every test opts in through recordExec instead
func TestMain(m *testing.M) {
	replace = func(_ string, argv, _ []string) int { panic(fmt.Sprintf("a test reached a real exec: %q", argv)) }
	launchRun = func(_ string, args []string) int { panic(fmt.Sprintf("a test reached the real launcher: %q", args)) }
	os.Exit(m.Run())
}

type call struct {
	cwd  string
	argv []string
}

func recordExec(t *testing.T) *[]call {
	var calls []call
	orig := replace
	replace = func(_ string, argv, _ []string) int {
		cwd, _ := os.Getwd()
		calls = append(calls, call{cwd, argv})
		return 0
	}
	t.Cleanup(func() { replace = orig })
	return &calls
}

type stubSource struct {
	latest, got string
	asked       int
}

func (s *stubSource) Latest(context.Context) (string, error) {
	s.asked++
	return s.latest, nil
}

func (s *stubSource) Create(_ context.Context, version, dir string) error {
	s.got = version
	return makeCheckout(dir)
}

func useSource(t *testing.T, s *stubSource) {
	orig := newSource
	newSource = func(string) project.Source { return s }
	t.Cleanup(func() { newSource = orig })
}

func makeCheckout(root string) error {
	for _, f := range []string{"skelet", "bin/_lib", "cmd/skelet/go.mod"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func newCheckout(t *testing.T) string {
	root := tempDir(t)
	if err := makeCheckout(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func tempDir(t *testing.T) string {
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestInsideAProjectTheProjectAnswers(t *testing.T) {
	root := newCheckout(t)
	t.Chdir(filepath.Join(root, "cmd", "skelet"))
	for _, args := range [][]string{{"up", "--lean"}, {"help"}, {}, {"new", "channel", "eu"}, {"doctor"}} {
		calls := recordExec(t)
		if code := Main("v1.2.3", args); code != 0 {
			t.Fatalf("%q: exit %d", args, code)
		}
		want := append([]string{"bash", filepath.Join(root, "skelet")}, args...)
		if len(*calls) != 1 || !slices.Equal((*calls)[0].argv, want) {
			t.Errorf("%q: exec'd %v, want %q", args, *calls, want)
		}
	}
}

func TestVersionIsTheInstalledBinarysOwn(t *testing.T) {
	root := newCheckout(t)
	t.Chdir(root)
	calls := recordExec(t)
	if code := Main("v1.2.3", []string{"version"}); code != 0 || len(*calls) != 0 {
		t.Errorf("exit %d, exec'd %v", code, *calls)
	}
}

func TestOutsideAProjectOnlyNewVersionAndHelpWork(t *testing.T) {
	t.Chdir(tempDir(t))
	calls := recordExec(t)
	for args, want := range map[string]int{"up": 2, "logs app": 2, "new": 2, "new channel eu": 2, "help": 0, "--help": 0, "": 0} {
		if code := Main("v1.2.3", strings.Fields(args)); code != want {
			t.Errorf("%q: exit %d, want %d", args, code, want)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("exec'd %v", *calls)
	}
}

// Outside a project doctor checks the machine - with nothing on the PATH that's a failed check, not a refusal
func TestDoctorRunsOutsideAProject(t *testing.T) {
	t.Chdir(tempDir(t))
	t.Setenv("PATH", "")
	calls := recordExec(t)
	if code := Main("v1.2.3", []string{"doctor"}); code != 1 || len(*calls) != 0 {
		t.Errorf("exit %d, exec'd %v - want the machine check failing on no docker", code, *calls)
	}
}

// A release build that ended up in a checkout's CLI cache would hand off to the shim, which runs it again - so it
// refuses, from inside a project, where handing off is what it would otherwise do
func TestReleaseBuildRefusesToRunUnderTheShim(t *testing.T) {
	t.Chdir(newCheckout(t))
	calls := recordExec(t)
	t.Setenv("SKELET_SHIM", "1")
	if code := Main("v1.0.0", []string{"up"}); code != 1 || len(*calls) != 0 {
		t.Errorf("exit %d, exec'd %v - want 1 and nothing run", code, *calls)
	}
}

func TestNewFetchesItsOwnReleaseAndRunsSetupInsideIt(t *testing.T) {
	base := tempDir(t)
	t.Chdir(base)
	src := &stubSource{latest: "v9.9.9"}
	useSource(t, src)
	calls := recordExec(t)
	if code := Main("v1.2.3", []string{"new", "shop", "--yes", "--apps", "dashboard"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if src.got != "v1.2.3" || src.asked != 0 {
		t.Errorf("fetched %q after %d Latest calls, want the binary's own v1.2.3 and none", src.got, src.asked)
	}
	dir := filepath.Join(base, "shop")
	want := []string{"bash", filepath.Join(dir, "skelet"), "setup", "--yes", "--apps", "dashboard"}
	if len(*calls) != 1 || (*calls)[0].cwd != dir || !slices.Equal((*calls)[0].argv, want) {
		t.Errorf("exec'd %v, want %q from %s", *calls, want, dir)
	}
}

func TestPredatesCLI(t *testing.T) {
	old, current := tempDir(t), newCheckout(t)
	if !predatesCLI(old) || predatesCLI(current) {
		t.Errorf("predatesCLI: a tree without cmd/skelet = %v, a current one = %v", predatesCLI(old), predatesCLI(current))
	}
}

func TestNewFromADevBuildTakesTheNewestRelease(t *testing.T) {
	t.Chdir(tempDir(t))
	src := &stubSource{latest: "v9.9.9"}
	useSource(t, src)
	recordExec(t)
	if code := Main("dev", []string{"new", "shop"}); code != 0 || src.got != "v9.9.9" {
		t.Errorf("exit %d, fetched %q", code, src.got)
	}
}

func TestNewWontNestAProjectWithoutForce(t *testing.T) {
	root := newCheckout(t)
	t.Chdir(root)
	useSource(t, &stubSource{})
	calls := recordExec(t)
	if code := Main("v1.2.3", []string{"new", "shop"}); code != 1 || len(*calls) != 0 {
		t.Errorf("exit %d, exec'd %v", code, *calls)
	}
	if _, err := os.Stat(filepath.Join(root, "shop")); err == nil {
		t.Error("the refused project was created anyway")
	}
	if code := Main("v1.2.3", []string{"new", "shop", "--force"}); code != 0 || len(*calls) != 1 {
		t.Errorf("--force: exit %d, exec'd %v", code, *calls)
	}
}

func newNative(t *testing.T) string {
	t.Helper()
	root := tempDir(t)
	p := filepath.Join(root, "cmd", "skelet", "go.mod")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("module github.com/skeletg/skelet/cmd/skelet\n\ngo 1.27.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// A project with no shim gets its CLI built and run by the launcher, args and all
func TestANativeProjectIsLaunched(t *testing.T) {
	root := newNative(t)
	t.Chdir(filepath.Join(root, "cmd"))
	var got []string
	orig := launchRun
	launchRun = func(r string, args []string) int { got = append([]string{r}, args...); return 0 }
	t.Cleanup(func() { launchRun = orig })
	calls := recordExec(t)
	if code := Main("v1.2.3", []string{"up", "--lean"}); code != 0 || len(*calls) != 0 {
		t.Fatalf("exit %d, exec'd %v", code, *calls)
	}
	if want := []string{root, "up", "--lean"}; !slices.Equal(got, want) {
		t.Errorf("launched %v, want %v", got, want)
	}
}

func TestALaunchedReleaseBuildRefuses(t *testing.T) {
	t.Chdir(newCheckout(t))
	calls := recordExec(t)
	t.Setenv("SKELET_LAUNCHED", "1")
	if code := Main("v1.0.0", []string{"up"}); code != 1 || len(*calls) != 0 {
		t.Errorf("exit %d, exec'd %v - want 1 and nothing run", code, *calls)
	}
}
