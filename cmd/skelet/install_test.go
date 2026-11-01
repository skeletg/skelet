package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const fakeCLI = "#!/bin/sh\necho skelet v0.0.0-test\n"

// A release's download directory, served the way GitHub serves one
func release(t *testing.T, checksum string) string {
	dist := t.TempDir()
	asset := "skelet-" + runtime.GOOS + "-" + runtime.GOARCH
	if checksum == "" {
		sum := sha256.Sum256([]byte(fakeCLI))
		checksum = hex.EncodeToString(sum[:])
	}
	for name, body := range map[string]string{asset: fakeCLI, "checksums.txt": checksum + "  " + asset + "\n"} {
		if err := os.WriteFile(filepath.Join(dist, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dist)))
	t.Cleanup(srv.Close)
	return srv.URL
}

func install(t *testing.T, url, dir string, args ...string) (string, error) {
	cmd := exec.Command("sh", append([]string{"install.sh"}, args...)...)
	cmd.Env = append(os.Environ(), "SKELET_DOWNLOAD_URL="+url, "SKELET_INSTALL_DIR="+dir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func needInstallTools(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("install.sh is for macOS and Linux")
	}
	_, curl := exec.LookPath("curl")
	_, wget := exec.LookPath("wget")
	if curl != nil && wget != nil {
		t.Skip("install.sh needs curl or wget")
	}
}

func TestInstallerChecksAndInstalls(t *testing.T) {
	needInstallTools(t)
	dir := filepath.Join(t.TempDir(), "bin")
	out, err := install(t, release(t, ""), dir)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	fi, err := os.Stat(filepath.Join(dir, "skelet"))
	if err != nil || fi.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed file: %v, mode %v", err, fi)
	}
	if !strings.Contains(out, "installed skelet v0.0.0-test") || !strings.Contains(out, "isn't on your PATH") {
		t.Errorf("output:\n%s", out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left behind in %s: %v", dir, entries)
	}
}

func TestInstallerRefusesABadChecksum(t *testing.T) {
	needInstallTools(t)
	dir := filepath.Join(t.TempDir(), "bin")
	out, err := install(t, release(t, strings.Repeat("0", 64)), dir)
	if err == nil || !strings.Contains(out, "does not match its checksum") {
		t.Errorf("err %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "skelet")); err == nil {
		t.Error("installed a download that failed its checksum")
	}
}

func TestInstallerFailsWhenTheReleaseHasNoBuild(t *testing.T) {
	needInstallTools(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	if out, err := install(t, srv.URL, t.TempDir()); err == nil {
		t.Errorf("succeeded against an empty release:\n%s", out)
	}
}

// A checkout's ./skelet symlinked onto the PATH is somebody's working setup - replacing it takes --force
func TestInstallerKeepsASymlinkWithoutForce(t *testing.T) {
	needInstallTools(t)
	dir := t.TempDir()
	link := filepath.Join(dir, "skelet")
	if err := os.Symlink("/somewhere/skelet/skelet", link); err != nil {
		t.Fatal(err)
	}
	url := release(t, "")
	if out, err := install(t, url, dir); err == nil || !strings.Contains(out, "--force") {
		t.Errorf("err %v\n%s", err, out)
	}
	if dest, err := os.Readlink(link); err != nil || dest != "/somewhere/skelet/skelet" {
		t.Errorf("symlink now %q, %v", dest, err)
	}
	if out, err := install(t, url, dir, "--force"); err != nil {
		t.Fatalf("--force: %v\n%s", err, out)
	}
	if fi, err := os.Lstat(link); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("after --force: %v, %v", fi, err)
	}
}
