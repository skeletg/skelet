package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCurrentReadsTheClonesEnv(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".env": "SESSION_KEY=x\nSKELET_TARGET=dev\nSKELET_APPS=dashboard,shop\nCOMPOSE_PROFILES=search,dashboard,shop,mailpit,signals\n"})
	p := Current(root)
	for k, want := range map[string]string{TargetKey: "dev", AppsKey: "dashboard,shop", DockerKey: "dashboard,shop", ToolsKey: "mailpit"} {
		if p[k] != want {
			t.Errorf("%s = %q, want %q", k, p[k], want)
		}
	}
	bare := t.TempDir()
	writeFiles(t, bare, map[string]string{".env": "SESSION_KEY=x\n"})
	if p := Current(bare); len(p) != 0 {
		t.Errorf("a .env that records nothing should answer nothing, got %v", p)
	}
}

func TestCurrentKeepsInfraOutOfTheApps(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".env": "COMPOSE_PROFILES=bundled,edge,nextjs-vetrina,queue\n"})
	if p := Current(root); p[DockerKey] != "nextjs-vetrina" || p[ToolsKey] != "" {
		t.Errorf("got docker %q tools %q", p[DockerKey], p[ToolsKey])
	}
}

func TestDevProfilesStartWhatSkeletUpWould(t *testing.T) {
	if got := DevProfiles([]string{"dashboard"}, []string{"mailpit"}, true); got != "search,dashboard,mailpit,signals" {
		t.Errorf("got %q", got)
	}
	if got := DevProfiles(nil, nil, false); got != "search" {
		t.Errorf("got %q", got)
	}
}

func TestMigrateMovesTheRootFilesIntoEnv(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".env":            "SESSION_KEY=keep-me\n",
		"env/go.env":      "SIGNALS_ENABLED=true\n",
		".skelet-profile": "SKELET_TARGET=dev\nSKELET_SIGNALS=true\nSKELET_APPS_SELECTED=dashboard,vetrina\nSKELET_DOCKER_APPS=dashboard,gone-shop\n",
		".skelet-apps":    "dashboard, vetrina\n",
		".skelet-tools":   "mailpit\n",
	})
	if moved, err := Migrate(root); err != nil || !moved {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	raw, _ := os.ReadFile(EnvPath(root))
	for _, want := range []string{"SESSION_KEY=keep-me", "SKELET_TARGET=dev", "SKELET_APPS=dashboard,vetrina", "COMPOSE_PROFILES=search,dashboard,vetrina,mailpit,signals"} {
		if !strings.Contains(string(raw), want+"\n") {
			t.Errorf(".env should have %s:\n%s", want, raw)
		}
	}
	for _, f := range oldFiles {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			t.Errorf("%s should be gone", f)
		}
	}
	if fi, _ := os.Stat(EnvPath(root)); fi.Mode().Perm() != 0o600 {
		t.Errorf(".env holds secrets and should stay 0600, got %v", fi.Mode().Perm())
	}
	if moved, err := Migrate(root); moved || err != nil {
		t.Errorf("a second run should find nothing, got %v, %v", moved, err)
	}
}

func TestMigrateLeavesWhatEnvAlreadySays(t *testing.T) {
	server := t.TempDir()
	writeFiles(t, server, map[string]string{".env": "COMPOSE_PROFILES=bundled,shop\n", ".skelet-profile": "SKELET_TARGET=prod\n", ".skelet-apps": "shop\n"})
	if _, err := Migrate(server); err != nil {
		t.Fatal(err)
	}
	if p := Current(server); p[TargetKey] != "prod" || p[DockerKey] != "shop" {
		t.Errorf("server: %v", p)
	}
	newer := t.TempDir()
	writeFiles(t, newer, map[string]string{".env": "SKELET_TARGET=small\n", ".skelet-profile": "SKELET_TARGET=dev\n", ".skelet-apps": "dashboard\n"})
	if _, err := Migrate(newer); err != nil {
		t.Fatal(err)
	}
	if p := Current(newer); p[TargetKey] != "small" || p[DockerKey] != "" {
		t.Errorf("a newer .env should win: %v", p)
	}
	if _, err := os.Stat(filepath.Join(newer, ".skelet-apps")); err == nil {
		t.Error("the stale root file should be cleared")
	}
	bare := t.TempDir()
	writeFiles(t, bare, map[string]string{".env": "SESSION_KEY=x\n", ".skelet-profile": "SKELET_TARGET=small\n", ".skelet-apps": "shop\n"})
	if _, err := Migrate(bare); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := envfile.Get(EnvPath(bare), EnvProfiles); ok {
		t.Error("a server got dev's COMPOSE_PROFILES")
	}
	picked := t.TempDir()
	writeFiles(t, picked, map[string]string{".env": "COMPOSE_PROFILES=search,dashboard\n", ".skelet-apps": "dashboard,shop\n"})
	if _, err := Migrate(picked); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := envfile.Get(EnvPath(picked), EnvProfiles); v != "search,dashboard" {
		t.Errorf("COMPOSE_PROFILES already in .env got rewritten: %q", v)
	}
}
