// Package node answers what setup and doctor both ask of a frontend run on the host: which pnpm it pins, whether
// the pnpm here can run it, and what its node_modules holds
package node

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var pin = regexp.MustCompile(`"packageManager":\s*"pnpm@([0-9.]+)`)

// Pin is the pnpm version a package.json's packageManager names, or ""
func Pin(manifest []byte) string {
	if m := pin.FindSubmatch(manifest); m != nil {
		return string(m[1])
	}
	return ""
}

// Pnpm says what pnpm answers in dir and why it can't run an app pinned to want, or no problem when it can. It asks
// from inside the app, where pnpm switches itself to the pinned version - that switch is what breaks. With dir empty
// it only checks there is a pnpm at all
func Pnpm(root, dir, want string) (have, problem string) {
	if _, err := exec.LookPath("pnpm"); err != nil {
		if want == "" {
			want = "<version>"
		}
		return "", "no pnpm on PATH - corepack enable, or npm install -g pnpm@" + want
	}
	if dir == "" || want == "" {
		return "", ""
	}
	cmd := exec.Command("pnpm", "--version")
	cmd.Dir = filepath.Join(root, dir)
	raw, _ := cmd.Output()
	if have = strings.TrimSpace(string(raw)); have != want {
		answered := have
		if answered == "" {
			answered = "nothing"
		}
		return have, "pnpm in " + dir + " answers " + answered + ", the apps pin " + want + " - pnpm self-update " + want
	}
	return have, ""
}

// Tree is what a frontend's node_modules holds on the host: none, pnpm, or npm - the last an install from before the
// switch to pnpm, which pnpm won't take over without a terminal to confirm. Empty is none: a container mounting its
// volume at node_modules leaves an empty directory behind on the host
func Tree(dir string) string {
	entries, _ := os.ReadDir(filepath.Join(dir, "node_modules"))
	switch {
	case len(entries) == 0:
		return "none"
	case exists(filepath.Join(dir, "node_modules", ".modules.yaml")):
		return "pnpm"
	}
	return "npm"
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
