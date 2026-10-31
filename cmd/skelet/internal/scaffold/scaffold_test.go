package scaffold

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/verbtest"
)

// The goldens: testdata/in holds the tracked compose files as they stood when the bash scaffolder was retired, and
// testdata/want what that scaffolder made of them - byte for byte, but for the override header naming the command

func TestMain(m *testing.M) {
	// The confirmation reads SCAFFOLD_TTY instead of the terminal a test run would otherwise hand down
	openTTY = func() (io.ReadCloser, error) {
		if s, ok := os.LookupEnv("SCAFFOLD_TTY"); ok {
			return io.NopCloser(strings.NewReader(s)), nil
		}
		return nil, errors.New("no terminal")
	}
	verbtest.Main(m, map[string]func(string, []string) int{"add": Add, "rm": Remove})
}

func testdata(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func put(t *testing.T, root, rel, body string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// clone is a committed checkout holding the golden inputs and a storefront to fork
func clone(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{devFile, prodFile, smallFile} {
		put(t, root, f, testdata(t, "in/"+f), 0o644)
	}
	for _, f := range []string{"prod-vetrina-demo.env.example", "small-vetrina-demo.env.example"} {
		put(t, root, "env/"+f, testdata(t, "in/"+f), 0o644)
	}
	app := "apps/channels/vetrina-demo/"
	put(t, root, app+"package.json", `{"name": "vetrina-demo"}`+"\n", 0o644)
	put(t, root, app+"scripts/run.sh", "#!/bin/sh\necho vetrina-demo\n", 0o775)
	// Past the umask, the way cp -a copies it
	if err := os.Chmod(filepath.Join(root, app+"scripts/run.sh"), 0o775); err != nil {
		t.Fatal(err)
	}
	put(t, root, app+"public/icon.bin", "\x00vetrina-demo\x00", 0o644)
	put(t, root, app+"node_modules/x/index.js", "vetrina-demo", 0o644)
	put(t, root, app+".next/cache", "vetrina-demo", 0o644)
	if err := os.Symlink("package.json", filepath.Join(root, app+"link.json")); err != nil {
		t.Fatal(err)
	}
	put(t, root, ".gitignore", "node_modules\n.next\ndocker-compose.override.yml\n", 0o644)
	git(t, root, "init", "-q")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "base")
	return root
}

func file(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func exists(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, rel))
	return err == nil
}

func run(t *testing.T, verb, root string, args ...string) (string, int) {
	t.Helper()
	return verbtest.NewDocker(t, "").Run(verb, root, args...)
}

func TestAddToTheOverride(t *testing.T) {
	root := clone(t)
	out, code := run(t, "add", root, "channel", "shop")
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if got := file(t, root, overrideFile); got != testdata(t, "want/fresh.override.yml") {
		t.Errorf("a new override differs from the golden:\n%s", got)
	}
	for _, f := range []string{"prod-shop.env.example", "small-shop.env.example"} {
		if file(t, root, "env/"+f) != testdata(t, "want/"+f) {
			t.Errorf("env/%s differs from the golden", f)
		}
	}
	// The fork: renamed through its text files, binaries as they were, the source's install and build left behind
	app := "apps/channels/shop/"
	if file(t, root, app+"package.json") != `{"name": "shop"}`+"\n" || file(t, root, app+"public/icon.bin") != "\x00vetrina-demo\x00" {
		t.Error("the fork wasn't renamed through its text files only")
	}
	if exists(root, app+"node_modules") || exists(root, app+".next") {
		t.Error("the fork carried the source's node_modules or .next")
	}
	if fi, err := os.Stat(filepath.Join(root, app+"scripts/run.sh")); err != nil || fi.Mode().Perm() != 0o775 {
		t.Errorf("the fork lost a script's mode: %v", err)
	}
	if link, err := os.Readlink(filepath.Join(root, app+"link.json")); err != nil || link != "package.json" {
		t.Errorf("the fork didn't keep a symlink: %q %v", link, err)
	}
	if !strings.Contains(out, "skelet setup --apps dashboard,vetrina-demo,shop") || !strings.Contains(out, "http://localhost:3001") {
		t.Errorf("next steps:\n%s", out)
	}

	// A second shop splices into the override that's there, on the next free port
	if _, code := run(t, "add", root, "channel", "two"); code != 0 || file(t, root, overrideFile) != testdata(t, "want/second.override.yml") {
		t.Errorf("second: exit %d\n%s", code, file(t, root, overrideFile))
	}
	// Removing one leaves the other - and the last one takes the emptied override with it
	t.Setenv("SCAFFOLD_TTY", "y\n")
	if _, code := run(t, "rm", root, "channel", "shop"); code != 0 || file(t, root, overrideFile) != testdata(t, "want/rm-one.override.yml") {
		t.Errorf("rm shop: exit %d\n%s", code, file(t, root, overrideFile))
	}
	if exists(root, "apps/channels/shop") || exists(root, "env/prod-shop.env.example") {
		t.Error("rm left the storefront's files")
	}
	if _, code := run(t, "rm", root, "channel", "two", "-y"); code != 0 || exists(root, overrideFile) {
		t.Errorf("rm the last: exit %d, override still there", code)
	}
}

func TestAddFromAnOverrideShop(t *testing.T) {
	root := clone(t)
	run(t, "add", root, "channel", "shop")
	if _, code := run(t, "add", root, "channel", "three", "--from", "shop"); code != 0 || file(t, root, overrideFile) != testdata(t, "want/from.override.yml") {
		t.Errorf("exit %d\n%s", code, file(t, root, overrideFile))
	}
}

// --tracked splices all three compose files, stages the env templates, and rm puts the files back exactly
func TestTracked(t *testing.T) {
	root := clone(t)
	if out, code := run(t, "add", root, "channel", "shop", "--tracked"); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, f := range []string{devFile, prodFile, smallFile} {
		if file(t, root, f) != testdata(t, "want/tracked."+f) {
			t.Errorf("%s differs from the golden", f)
		}
	}
	if staged := git(t, root, "diff", "--cached", "--name-only"); staged != "env/prod-shop.env.example\nenv/small-shop.env.example\n" {
		t.Errorf("staged: %q", staged)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "shop")
	if out, code := run(t, "rm", root, "channel", "shop", "-y"); code != 0 || !strings.Contains(out, "committed - recoverable") {
		t.Fatalf("rm: exit %d:\n%s", code, out)
	}
	for _, f := range []string{devFile, prodFile, smallFile} {
		if file(t, root, f) != testdata(t, "in/"+f) {
			t.Errorf("rm didn't restore %s", f)
		}
	}
	if staged := git(t, root, "diff", "--cached", "--name-only"); !strings.Contains(staged, "env/prod-shop.env.example") {
		t.Errorf("the env templates' removal isn't staged: %q", staged)
	}
}

// Overrides kept by hand: an anchor already there isn't copied twice, a section after services: or a comment at the
// margin is respected, and volumes: above services: gets the volumes rather than services: getting them
func TestAddIntoHandKeptOverrides(t *testing.T) {
	for in, want := range map[string]string{
		"anchored": "anchored", "networks": "networks", "comment-before-volumes": "commentvol", "volumes-first": "volfirst",
	} {
		root := clone(t)
		put(t, root, overrideFile, testdata(t, "in/override-"+in+".yml"), 0o644)
		if _, code := run(t, "add", root, "channel", "shop"); code != 0 || file(t, root, overrideFile) != testdata(t, "want/"+want+".override.yml") {
			t.Errorf("%s: exit %d\n%s", in, code, file(t, root, overrideFile))
		}
	}
	// With no services: to put it in, nothing is written - it used to add the volumes and drop the service
	root := clone(t)
	put(t, root, overrideFile, testdata(t, "in/override-no-services.yml"), 0o644)
	if _, code := run(t, "add", root, "channel", "shop"); code != 1 || file(t, root, overrideFile) != testdata(t, "in/override-no-services.yml") || exists(root, "apps/channels/shop") {
		t.Errorf("no services: exit %d", code)
	}
}

func TestAddRefusals(t *testing.T) {
	root := clone(t)
	put(t, root, overrideFile, "services:\n  taken:\n    image: x\n", 0o644)
	put(t, root, "apps/channels/vetrina.demo/package.json", "{}", 0o644)
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "odd name")
	for _, c := range []struct {
		args []string
		code int
	}{
		{[]string{"widget", "x"}, 2},
		{[]string{"channel"}, 2},
		{[]string{"channel", "Bad"}, 2},
		{[]string{"channel", "-shop"}, 2},
		{[]string{"channel", "shop-"}, 2},
		{[]string{"channel", "vetrina-demo"}, 2},
		// A service only the override declares is taken too
		{[]string{"channel", "taken"}, 2},
		// Even when there's a directory by that name - it would be matched as a pattern in every file it's copied to
		{[]string{"channel", "x", "--from", "vetrina.demo"}, 2},
		{[]string{"channel", "x", "--from", "nope"}, 2},
		{[]string{"channel", "x", "--from"}, 2},
		{[]string{"channel", "x", "--bogus"}, 2},
	} {
		if _, code := run(t, "add", root, c.args...); code != c.code {
			t.Errorf("%q: exit %d, want %d", c.args, code, c.code)
		}
	}
	if git(t, root, "status", "--porcelain") != "" || len(entries(t, root, "apps/channels")) != 2 {
		t.Errorf("a refusal wrote something:\n%s", git(t, root, "status", "--porcelain"))
	}
	// compose rejecting the result writes nothing either
	broken := verbtest.NewDocker(t, `case "$*" in *config*) echo "services.shop must be a mapping" >&2; exit 1 ;; esac`)
	if _, code := broken.Run("add", root, "channel", "shop"); code != 1 || exists(root, "apps/channels/shop") || file(t, root, overrideFile) != "services:\n  taken:\n    image: x\n" {
		t.Errorf("invalid result: exit %d", code)
	}
}

func entries(t *testing.T, root, rel string) []os.DirEntry {
	t.Helper()
	es, err := os.ReadDir(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestRemoveRefusals(t *testing.T) {
	root := clone(t)
	run(t, "add", root, "channel", "shop")
	t.Setenv("SCAFFOLD_TTY", "n\n")
	if out, code := run(t, "rm", root, "channel", "shop"); code != 1 || !strings.Contains(out, "aborted") || !exists(root, "apps/channels/shop") {
		t.Errorf("answering no: exit %d\n%s", code, out)
	}
	_ = os.Unsetenv("SCAFFOLD_TTY")
	if _, code := run(t, "rm", root, "channel", "shop"); code != 1 || !exists(root, "apps/channels/shop") {
		t.Errorf("no terminal: exit %d", code)
	}
	for _, args := range [][]string{{"channel", "nothing", "-y"}, {"channel", "Bad", "-y"}, {"widget", "x"}, {"channel", "shop", "--bogus"}} {
		if _, code := run(t, "rm", root, args...); code == 0 || !exists(root, "apps/channels/shop") {
			t.Errorf("%q: exit %d", args, code)
		}
	}
}

// git can't bring back an untracked directory, nor untracked files inside a tracked one
func TestRecoverable(t *testing.T) {
	root := clone(t)
	dir := "apps/channels/vetrina-demo"
	if got := recoverable(root, dir); got != "committed - recoverable with git checkout" {
		t.Errorf("clean: %s", got)
	}
	put(t, root, dir+"/notes.txt", "mine", 0o644)
	if got := recoverable(root, dir); !strings.Contains(got, "UNTRACKED files") {
		t.Errorf("an untracked file inside: %s", got)
	}
	put(t, root, dir+"/package.json", "{}", 0o644)
	if got := recoverable(root, dir); !strings.Contains(got, "UNCOMMITTED changes") {
		t.Errorf("modified: %s", got)
	}
	put(t, root, "apps/channels/loose/package.json", "{}", 0o644)
	if got := recoverable(root, "apps/channels/loose"); !strings.HasPrefix(got, "untracked") {
		t.Errorf("untracked: %s", got)
	}
}

func TestDeselect(t *testing.T) {
	root := t.TempDir()
	put(t, root, ".skelet-apps", "dashboard,,eu-shop,shop\n", 0o644)
	if err := deselect(root, "shop"); err != nil || file(t, root, ".skelet-apps") != "dashboard,eu-shop\n" {
		t.Errorf("%v %q", err, file(t, root, ".skelet-apps"))
	}
}

func TestFreePort(t *testing.T) {
	files := `ports: ["3001:3000"]` + "\nURL=http://localhost:3002\n" + `- '3004:3000'` + "\n30030\n"
	if p := freePort(3001, files); p != 3003 {
		t.Errorf("free port %d, want 3003", p)
	}
}

// compose rejecting what rm would leave writes and deletes nothing
func TestRemoveValidates(t *testing.T) {
	root := clone(t)
	run(t, "add", root, "channel", "shop")
	run(t, "add", root, "channel", "two")
	before := file(t, root, overrideFile)
	broken := verbtest.NewDocker(t, `case "$*" in *config*) exit 1 ;; esac`)
	if _, code := broken.Run("rm", root, "channel", "shop", "-y"); code != 1 || file(t, root, overrideFile) != before || !exists(root, "apps/channels/shop") {
		t.Errorf("invalid result: exit %d", code)
	}
}

func TestSpliceAfterTheLastService(t *testing.T) {
	got := spliceAfter("services:\n  a:\n    image: a\n", "a", []string{"  b:", "    image: b"})
	if got != "services:\n  a:\n    image: a\n  b:\n    image: b\n" {
		t.Errorf("%q", got)
	}
}

// A comment or a blank line under services: isn't a service
func TestHasServices(t *testing.T) {
	for text, want := range map[string]bool{
		"services:\n\n  # mine\nvolumes:\n  x:\n": false,
		"services:\n  app:\n    image: x\n":       true,
		"volumes:\n  app:\n":                      false,
	} {
		if hasServices(text) != want {
			t.Errorf("%q: want %v", text, want)
		}
	}
}
