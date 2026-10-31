package route

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Every route to a script, run through the bash dispatcher this replaced and through Resolve against stub scripts that
// echo how they were called. Help is left out on purpose - its screen names `add` where the old one said `new` - and
// so are setup and doctor, which are Go now
func TestMatchesTheBashDispatcher(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash to run the old dispatcher with")
	}
	root, err := filepath.EvalSymlinks(fakeRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := os.ReadFile("testdata/legacy-skelet.bash")
	if err != nil {
		t.Fatal(err)
	}
	write := func(path string, body []byte, mode os.FileMode) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "skelet"), legacy, 0o755)
	write(filepath.Join(root, "bin", "_lib"), nil, 0o644)
	for _, r := range routes {
		if r.Script == "" {
			continue
		}
		write(filepath.Join(root, "bin", r.Script), []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" \"$@\"\n"), 0o755)
	}

	cases := [][]string{
		{"up", "--lean"}, {"serve", "dashboard"}, {"down"}, {"stop"}, {"restart"}, {"ps"}, {"logs", "app"},
		{"sh"}, {"go", "test", "./..."}, {"cli", "admin", "list"}, {"psql"}, {"lint"}, {"search", "status"},
		{"release"}, {"fixperms"}, {"provider", "u"}, {"changelog", "check"},
		{"rm", "channel", "x"}, {"new", "channel", "x", "--from", "vetrina"},
		{"pnpm", "dashboard", "run", "lint"}, {"pnpm", "vetrina", "i"}, {"pnpm"}, {"pnpm", "nope", "x"},
		{"npm", "dashboard", "x"}, {"npm"},
		{"db", "migrate", "up"}, {"db", "migrate", "--signals", "status"}, {"db", "migrate", "new", "x", "--signals"},
		{"db", "export", "f"}, {"db", "import", "f"}, {"db", "shell"}, {"db"}, {"db", "nope"},
		{"gen"}, {"gen", "--check"}, {"gen", "sqlc"}, {"gen", "gqlgen", "x"}, {"gen", "buf"},
		{"check"}, {"check", "--codegen"}, {"check", "pins"}, {"check", "structure"}, {"check", "contracts"},
		{"check", "codegen", "--no-frontend"},
		{"test"}, {"test", "--changed=HEAD"}, {"test", "e2e", "-run", "X"}, {"test", "storefront"}, {"test", "./internal/x"},
		{"nope"}, {"nope", "x"},
	}
	for _, args := range cases {
		cmd := exec.Command(bash, append([]string{filepath.Join(root, "skelet")}, args...)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "PWD="+root)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		code := 0
		if err := cmd.Run(); err != nil {
			ee, ok := errors.AsType[*exec.ExitError](err)
			if !ok {
				t.Fatalf("%q: %v", args, err)
			}
			code = ee.ExitCode()
		}

		a := Resolve(root, args)
		// A verb ported to Go no longer runs a script, so there's nothing left to compare - its own tests cover it
		if a.Go != nil {
			continue
		}
		gotOut, gotErr := a.Stdout, ""
		if a.Script != "" {
			gotOut = strings.Join(append([]string{filepath.Join(root, "bin", a.Script)}, a.Args...), "\n") + "\n"
		}
		if a.Notice != "" && !strings.HasPrefix(a.Notice, "(skelet new channel") {
			gotErr += a.Notice + "\n"
		}
		if a.Stderr != "" {
			gotErr += a.Stderr + "\n"
		}
		if gotOut != out.String() || gotErr != errb.String() || a.Code != code {
			t.Errorf("%q differs\n bash: exit %d out %q err %q\n   go: exit %d out %q err %q",
				args, code, out.String(), errb.String(), a.Code, gotOut, gotErr)
		}
	}
}
