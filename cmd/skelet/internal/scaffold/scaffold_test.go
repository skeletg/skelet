package scaffold

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
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

// templateClone is a checkout in the workspace layout: vetrina a template, wired the way the tracked files wire it
func templateClone(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, devFile, `x-logging: &logging
  driver: json-file

services:
  vetrina:
    profiles: ["vetrina"]
    build:
      context: .
      dockerfile: templates/vetrina/Dockerfile
    ports:
      - "3000:3000"
    volumes:
      - .:/var/www/skelet
      - vetrina-node-modules:/var/www/skelet/node_modules
      - vetrina-next-cache:/var/www/skelet/templates/vetrina/.next
    logging: *logging

volumes:
  vetrina-node-modules:
  vetrina-next-cache:
`, 0o644)
	put(t, root, prodFile, `services:
  vetrina:
    image: ${VETRINA_IMAGE:-skelet-vetrina:prod}
    build:
      context: .
      dockerfile: templates/vetrina/Dockerfile.prod
    env_file: [env/prod-vetrina.env]
    ports: ["127.0.0.1:3000:3000"]
`, 0o644)
	put(t, root, smallFile, `services:
  vetrina:
    image: ${VETRINA_IMAGE:-skelet-vetrina:prod}
    env_file: [env/small-vetrina.env]
    ports: ["127.0.0.1:13000:3000"]
`, 0o644)
	put(t, root, "env/prod-vetrina.env.example", "STOREFRONT_REVALIDATE_URL=http://vetrina:3000\n", 0o644)
	put(t, root, "pnpm-workspace.yaml", "packages:\n  - apps/channels/*\n  - packages/*\n  - templates/*\n", 0o644)
	put(t, root, "packages/storefront-next/package.json", `{"name": "@skelet/storefront-next", "packageManager": "pnpm@12.6.0"}`+"\n", 0o644)
	put(t, root, ".nvmrc", "24\n", 0o644)
	put(t, root, "pnpm-lock.yaml", "lockfileVersion: '9.0'\n", 0o644)
	tpl := "templates/vetrina/"
	put(t, root, tpl+"package.json", `{"name": "vetrina"}`+"\n", 0o644)
	put(t, root, tpl+"Dockerfile", "COPY templates/vetrina/package.json templates/vetrina/\nRUN pnpm install --filter \"./templates/vetrina...\"\n", 0o644)
	put(t, root, tpl+"Dockerfile.prod.dockerignore", "*\n!packages/\n!templates/vetrina/\n", 0o644)
	put(t, root, ".gitignore", "node_modules\n.next\ndocker-compose.override.yml\n", 0o644)
	git(t, root, "init", "-q")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "base")
	// What an old per-app install leaves behind - a fork carrying it would be a pnpm project apart from the workspace
	put(t, root, tpl+"pnpm-lock.yaml", "lockfileVersion: '9.0'\n", 0o644)
	return root
}

// lockfileDocker writes the files pnpm was shown into the lockfile, one a line
var lockfileDocker = `case "$*" in *--lockfile-only*)
  for a in "$@"; do case "$a" in *:/w) d="${a%:/w}"; (cd "$d" && find . -type f | sed 's|^./||' | sort) > "$d/pnpm-lock.yaml";; esac; done;;
esac`

var scratchMount = regexp.MustCompile(`-v (\S+)/\.skelet/tmp/pnpm-[0-9]+:/w -w /w node:24-alpine npx -y pnpm@12\.6\.0 install --lockfile-only`)

func TestAddFromATemplate(t *testing.T) {
	root := templateClone(t)
	d := verbtest.NewDocker(t, lockfileDocker)
	if out, code := d.Run("add", root, "channel", "shop"); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	app := "apps/channels/shop/"
	if file(t, root, app+"package.json") != `{"name": "shop"}`+"\n" {
		t.Error("the fork wasn't renamed")
	}
	// The template's own path becomes the fork's - the workspace filter and the build context's whitelist name it
	if got := file(t, root, app+"Dockerfile"); got != "COPY apps/channels/shop/package.json apps/channels/shop/\nRUN pnpm install --filter \"./apps/channels/shop...\"\n" {
		t.Errorf("Dockerfile:\n%s", got)
	}
	if got := file(t, root, app+"Dockerfile.prod.dockerignore"); got != "*\n!packages/\n!apps/channels/shop/\n" {
		t.Errorf("dockerignore:\n%s", got)
	}
	if exists(root, app+"pnpm-lock.yaml") {
		t.Error("the fork carried the template's stray lockfile")
	}
	over := file(t, root, overrideFile)
	for _, want := range []string{
		`profiles: ["shop"]`, "dockerfile: apps/channels/shop/Dockerfile\n", `"3001:3000"`,
		"shop-node-modules:/var/www/skelet/node_modules", "shop-next-cache:/var/www/skelet/apps/channels/shop/.next",
		"  shop-node-modules:\n  shop-next-cache:\n",
	} {
		if !strings.Contains(over, want) {
			t.Errorf("the override lacks %q:\n%s", want, over)
		}
	}
	if strings.Contains(over, "vetrina") {
		t.Errorf("the override still names the template:\n%s", over)
	}
	if file(t, root, "env/prod-shop.env.example") != "STOREFRONT_REVALIDATE_URL=http://shop:3000\n" {
		t.Error("the env template wasn't renamed")
	}
	// The lockfile learns the new member in a throwaway container shown the workspace's manifests and nothing else
	calls := strings.Join(d.Calls(), "\n")
	if m := scratchMount.FindStringSubmatch(calls); m == nil || m[1] != root || !strings.Contains(calls, "run --rm") {
		t.Errorf("no lockfile refresh:\n%s", calls)
	}
	if got := file(t, root, "pnpm-lock.yaml"); got != "apps/channels/shop/package.json\npackages/storefront-next/package.json\npnpm-lock.yaml\npnpm-workspace.yaml\ntemplates/vetrina/package.json\n" {
		t.Errorf("pnpm was shown:\n%s", got)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".skelet/tmp")); len(entries) != 0 {
		t.Errorf("scratch left behind: %v", entries)
	}
	if st := git(t, root, "status", "--porcelain", "templates"); st != "?? templates/vetrina/pnpm-lock.yaml\n" {
		t.Errorf("the template changed:\n%s", st)
	}

	// With a storefront of the project's own beside it, the default is still the template
	put(t, root, app+"mine.txt", "mine", 0o644)
	if err := os.MkdirAll(filepath.Join(root, "templates/sketch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, code := run(t, "add", root, "channel", "two"); code != 0 || exists(root, "apps/channels/two/mine.txt") {
		t.Errorf("two: exit %d, or forked from shop", code)
	}
	if got := file(t, root, ".env"); got != "VITE_STOREFRONT_ORIGIN=http://localhost:3001\n" {
		t.Errorf("the dashboard's previews should stay on the first shop:\n%s", got)
	}
	// --from forks one of the project's own, --template only answers to a template, and the two don't mix
	if _, code := run(t, "add", root, "channel", "three", "--from", "shop"); code != 0 || !exists(root, "apps/channels/three/mine.txt") {
		t.Errorf("--from shop: exit %d", code)
	}
	for _, args := range [][]string{
		{"channel", "four", "--template", "shop"},
		{"channel", "four", "--template", "vetrina", "--from", "shop"},
		{"channel", "vetrina"},
		// A name a template directory holds, service or not - templates/sketch has neither a manifest nor a block
		{"channel", "sketch"},
	} {
		if _, code := run(t, "add", root, args...); code != 2 || exists(root, "apps/channels/four") {
			t.Errorf("%q: exit %d, want 2", args, code)
		}
	}
	if _, code := run(t, "add", root, "channel", "four", "--template", "vetrina"); code != 0 || exists(root, "apps/channels/four/mine.txt") {
		t.Errorf("--template vetrina: exit %d", code)
	}

	// rm takes the project's own and leaves the template be
	if _, code := run(t, "rm", root, "channel", "vetrina", "-y"); code != 2 || !exists(root, "templates/vetrina/package.json") {
		t.Errorf("rm a template: exit %d", code)
	}
	if _, code := run(t, "rm", root, "channel", "two", "-y"); code != 0 || exists(root, "apps/channels/two") {
		t.Errorf("rm two: exit %d", code)
	}
}

// --tracked from a template puts the fork beside it in all three files, its image and Dockerfile its own
func TestTrackedFromATemplate(t *testing.T) {
	root := templateClone(t)
	put(t, root, profile.File, profile.TargetKey+"=prod\n", 0o644)
	if out, code := run(t, "add", root, "channel", "shop", "--tracked"); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for f, want := range map[string][]string{
		devFile:   {"dockerfile: apps/channels/shop/Dockerfile\n", "  vetrina-next-cache:\n  shop-node-modules:\n  shop-next-cache:\n"},
		prodFile:  {"${SHOP_IMAGE:-skelet-shop:prod}", "dockerfile: apps/channels/shop/Dockerfile.prod", "env/prod-shop.env", "127.0.0.1:3001:3000"},
		smallFile: {"${SHOP_IMAGE:-skelet-shop:prod}", "env/small-shop.env", "127.0.0.1:13001:3000"},
	} {
		got := file(t, root, f)
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s lacks %q:\n%s", f, w, got)
			}
		}
		if !strings.Contains(got, "dockerfile: templates/vetrina/") && f != smallFile {
			t.Errorf("%s lost the template's own service", f)
		}
	}
	if exists(root, ".env") {
		t.Error("a server's .env got a localhost preview origin")
	}
	// A storefront in the tracked file doesn't take over as the default source - the template still is
	put(t, root, "apps/channels/shop/mine.txt", "mine", 0o644)
	if _, code := run(t, "add", root, "channel", "two"); code != 0 || exists(root, "apps/channels/two/mine.txt") {
		t.Errorf("two: exit %d, or forked from shop", code)
	}
}

// A template setup left out of the working tree is still forked - from git's HEAD, renamed the same way
func TestAddFromATemplateNotCheckedOut(t *testing.T) {
	root := templateClone(t)
	put(t, root, "templates/vetrina/scripts/run.sh", "#!/bin/sh\necho templates/vetrina\n", 0o755)
	if err := os.Symlink("package.json", filepath.Join(root, "templates/vetrina/link.json")); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A", "templates/vetrina/scripts", "templates/vetrina/link.json")
	git(t, root, "commit", "-qm", "more")
	git(t, root, "sparse-checkout", "set", "--no-cone", "/*", "!/templates/vetrina/")
	d := verbtest.NewDocker(t, lockfileDocker)
	if out, code := d.Run("add", root, "channel", "shop"); code != 0 || !strings.Contains(out, "from git") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if got := file(t, root, "pnpm-lock.yaml"); !strings.Contains(got, "templates/vetrina/package.json\n") {
		t.Errorf("the lockfile refresh lost the template setup left out:\n%s", got)
	}
	app := "apps/channels/shop/"
	if file(t, root, app+"package.json") != `{"name": "shop"}`+"\n" ||
		file(t, root, app+"Dockerfile.prod.dockerignore") != "*\n!packages/\n!apps/channels/shop/\n" ||
		file(t, root, app+"scripts/run.sh") != "#!/bin/sh\necho apps/channels/shop\n" {
		t.Error("the fork from git wasn't renamed through its text files")
	}
	if fi, err := os.Stat(filepath.Join(root, app+"scripts/run.sh")); err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("the fork from git lost a script's exec bit: %v", err)
	}
	if link, err := os.Readlink(filepath.Join(root, app+"link.json")); err != nil || link != "package.json" {
		t.Errorf("the fork from git didn't keep a symlink: %q %v", link, err)
	}
	if exists(root, "templates/vetrina/package.json") {
		t.Error("forking checked the template out")
	}
	// Named, and refused as a name of its own though it isn't on disk
	if _, code := run(t, "add", root, "channel", "two", "--template", "vetrina"); code != 0 || !exists(root, "apps/channels/two/Dockerfile") {
		t.Errorf("--template vetrina from git: exit %d", code)
	}
	if _, code := run(t, "add", root, "channel", "vetrina"); code != 2 {
		t.Errorf("a name only git has: exit %d, want 2", code)
	}
	if _, code := run(t, "rm", root, "channel", "vetrina", "-y"); code != 2 || !strings.Contains(file(t, root, prodFile), "  vetrina:") ||
		!exists(root, "env/prod-vetrina.env.example") {
		t.Errorf("rm a template setup left out: exit %d, or it took its wiring", code)
	}
	git(t, root, "sparse-checkout", "disable")
	if err := os.RemoveAll(filepath.Join(root, "templates/vetrina")); err != nil {
		t.Fatal(err)
	}
	if _, code := run(t, "add", root, "channel", "three", "--template", "vetrina"); code != 2 || exists(root, "apps/channels/three") {
		t.Errorf("forked a template deleted by hand: exit %d", code)
	}
}

func TestRenameIsWholeNamesOnly(t *testing.T) {
	m := newMove("apps/channels/shop", "shop", "euper")
	in := "workshop shop-header shop skelet-shop:prod env/prod-shop.env apps/channels/shop/x apps/channels/shopper " +
		"${SHOP_IMAGE} EU_SHOP_IMAGE eu-shop shop's"
	want := "workshop shop-header euper skelet-euper:prod env/prod-euper.env apps/channels/euper/x apps/channels/shopper " +
		"${EUPER_IMAGE} EU_SHOP_IMAGE eu-shop euper's"
	if got := m.applyFile(in); got != want {
		t.Errorf("applyFile:\n got %s\nwant %s", got, want)
	}
	if got := m.apply("  - shop-node-modules:/w shop-next-cache:/n shop-header"); got != "  - euper-node-modules:/w euper-next-cache:/n shop-header" {
		t.Errorf("apply: %s", got)
	}
}

func TestAddFailingLeavesNothing(t *testing.T) {
	root := templateClone(t)
	secret := filepath.Join(root, "templates/vetrina/unreadable.txt")
	put(t, root, "templates/vetrina/unreadable.txt", "x", 0o644)
	if err := os.Chmod(secret, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o644) })
	if _, err := os.ReadFile(secret); err == nil {
		t.Skip("running as a user who reads anything")
	}
	if out, code := run(t, "add", root, "channel", "shop"); code != 1 {
		t.Fatalf("exit %d, want 1:\n%s", code, out)
	}
	if exists(root, "apps/channels/shop") || exists(root, overrideFile) || exists(root, "env/prod-shop.env.example") {
		t.Error("a failed copy left part of the storefront behind")
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".skelet/tmp")); len(entries) != 0 {
		t.Errorf("scratch left behind: %v", entries)
	}
}

func TestAddLockfileFailureSaysSo(t *testing.T) {
	root := templateClone(t)
	d := verbtest.NewDocker(t, `case "$*" in *--lockfile-only*) exit 1;; esac`)
	out, code := d.Run("add", root, "channel", "shop")
	if code != 1 || !exists(root, "apps/channels/shop/package.json") {
		t.Fatalf("exit %d, want 1 with the storefront made:\n%s", code, out)
	}
}

func TestTrackedFromATemplateOnOtherPorts(t *testing.T) {
	root := templateClone(t)
	for f, swap := range map[string][2]string{
		devFile:   {`"3000:3000"`, `"3001:3000"`},
		prodFile:  {"127.0.0.1:3000:3000", "127.0.0.1:3001:3000"},
		smallFile: {"127.0.0.1:13000:3000", "127.0.0.1:13001:3000"},
	} {
		put(t, root, f, strings.Replace(file(t, root, f), swap[0], swap[1], 1), 0o644)
	}
	put(t, root, "env/small-vetrina.env.example", "STOREFRONT_BASE_URL=http://localhost:13001\n", 0o644)
	if out, code := run(t, "add", root, "channel", "shop", "--tracked"); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for f, want := range map[string]string{prodFile: "127.0.0.1:3002:3000", smallFile: "127.0.0.1:13002:3000"} {
		if got := block(file(t, root, f), "shop"); !strings.Contains(strings.Join(got, "\n"), want) {
			t.Errorf("%s's shop block lacks %s:\n%s", f, want, strings.Join(got, "\n"))
		}
	}
	if got := file(t, root, "env/small-shop.env.example"); got != "STOREFRONT_BASE_URL=http://localhost:13002\n" {
		t.Errorf("small env template: %s", got)
	}
}
