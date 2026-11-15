package stack

import (
	"github.com/skeletg/skelet/cmd/skelet/internal/node"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The root .env wins over the caller's shell, as set -a; . .env did, and the dev defaults only fill what's still
// missing or empty
func TestDevEnv(t *testing.T) {
	root := t.TempDir()
	body := "STOREFRONT_CHANNEL_ID=from-dotenv\nSHELL_SET=dotenv-wins\nNEXT_PUBLIC_BACKEND_URL=\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL_SET", "shell")
	t.Setenv("STOREFRONT_REVALIDATE_SECRET", "mine")
	t.Setenv("SKELET_ROOT", "/x")
	env, err := devEnv(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"STOREFRONT_CHANNEL_ID=from-dotenv", "SHELL_SET=dotenv-wins",
		"NEXT_PUBLIC_BACKEND_URL=http://localhost:8080", "STOREFRONT_REVALIDATE_SECRET=mine", "LC_ALL=C.UTF-8",
		"STOREFRONT_CHANNEL_ID=from-dotenv",
	} {
		if !slices.Contains(env, want) {
			t.Errorf("no %s in the dev server's environment", want)
		}
	}
	if slices.Contains(env, "SKELET_ROOT=/x") {
		t.Error("the CLI's own variable reached the dev server")
	}
}

func TestEachAppListensOnItsDevScriptsPort(t *testing.T) {
	for manifest, want := range map[string]string{
		`{"scripts": {"dev": "next dev -p 3002", "start": "next start -p 9"}}`: "3002",
		`{"scripts": {"dev": "astro dev --port 3001"}}`:                        "3001",
		`{"scripts": {"dev": "vite --port=5174"}}`:                             "5174",
		`{"scripts": {"dev": "vite"}}`:                                         "5173",
	} {
		if got := node.DevPort([]byte(manifest), "5173"); got != want {
			t.Errorf("%s: port %s, want %s", manifest, got, want)
		}
	}
}

func TestTwoAppsOnOnePortAreRefusedBeforeEitherStarts(t *testing.T) {
	if code := checkPorts([]devApp{{name: "shop", port: "39871"}, {name: "outlet", port: "39871"}}); code != 1 {
		t.Errorf("exit %d", code)
	}
	if code := checkPorts([]devApp{{name: "shop", port: "39871"}, {name: "outlet", port: "39872"}}); code != 0 {
		t.Errorf("distinct free ports: exit %d", code)
	}
}

func TestOneAppExitingStopsTheRest(t *testing.T) {
	root, bin := t.TempDir(), t.TempDir()
	for _, dir := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\ncase \"${PWD##*/}\" in a) echo hello-a; sleep 0.5; exit 3;; *) echo hello-b; exec sleep 30;; esac\n"
	if err := os.WriteFile(filepath.Join(bin, "pnpm"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	start := time.Now()
	code := together(root, []devApp{{name: "a", dir: "a", port: "1"}, {name: "bee", dir: "b", port: "2"}}, os.Environ())
	os.Stdout = stdout
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if code != 3 || time.Since(start) > 8*time.Second {
		t.Errorf("exit %d after %v - want a's 3, with b stopped rather than waited out", code, time.Since(start))
	}
	for _, want := range []string{"a   | hello-a", "bee | hello-b"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
}
