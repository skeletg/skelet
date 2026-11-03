package project

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// A bare remote shaped like Skelet's: release tags out of text order, a pre-release, a module tag, and an untagged tip
func upstream(t *testing.T) string {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	work := t.TempDir()
	git(t, work, "init", "-q", "-b", "develop")
	commit := func(file string, tags ...string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(work, file)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(work, file), []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
		git(t, work, "add", "-A")
		git(t, work, "commit", "-q", "-m", file)
		for _, tag := range tags {
			git(t, work, "tag", "-a", tag, "-m", tag)
		}
	}
	commit("skelet", "v0.0.1")
	commit("bin/_lib", "v0.0.2", "v0.0.10", "v0.1.0-rc1", "apps/backend/core/v9.0.0")
	commit("README.md")
	bare := filepath.Join(t.TempDir(), "skelet.git")
	git(t, "", "clone", "-q", "--bare", work, bare)
	git(t, bare, "config", "uploadpack.allowFilter", "true")
	return "file://" + bare
}

func TestGitLatestIsTheHighestRelease(t *testing.T) {
	src := Git{URL: upstream(t), Out: io.Discard}
	if got, err := src.Latest(context.Background()); err != nil || got != "v0.0.10" {
		t.Errorf("Latest = %q, %v - want v0.0.10", got, err)
	}
}

func TestGitCreateStartsMainAtTheRelease(t *testing.T) {
	url := upstream(t)
	dir := filepath.Join(t.TempDir(), "shop")
	if err := (Git{URL: url, Out: io.Discard}).Create(context.Background(), "v0.0.1", dir); err != nil {
		t.Fatal(err)
	}
	bare := strings.TrimPrefix(url, "file://")
	if got, want := git(t, dir, "rev-parse", "HEAD"), git(t, bare, "rev-parse", "v0.0.1^{commit}"); got != want {
		t.Errorf("HEAD %s, want v0.0.1's %s", got, want)
	}
	for args, want := range map[string]string{
		"rev-parse --abbrev-ref HEAD":               "main",
		"remote":                                    "upstream",
		"remote get-url upstream":                   url,
		"config remote.upstream.partialclonefilter": "blob:none",
	} {
		if got := git(t, dir, strings.Fields(args)...); got != want {
			t.Errorf("git %s = %q, want %q", args, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "bin", "_lib")); err == nil {
		t.Error("a file from after v0.0.1 is in the checkout")
	}
}

func TestGitCreateFailsOnAMissingRelease(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shop")
	if err := (Git{URL: upstream(t), Out: io.Discard}).Create(context.Background(), "v7.7.7", dir); err == nil {
		t.Error("created a project from a tag that doesn't exist")
	}
}
