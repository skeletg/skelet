// Package fixperms is skelet fixperms: chown files a container left owned by someone else back to you - under apps/, the
// templates, the shared packages and the workspace's node_modules
package fixperms

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const About = `Chowns files a container left owned by someone else back to you - under apps/, templates/,
packages/ and the workspace's node_modules.

  skelet fixperms          scan, list what's wrong, and ask for sudo only when there's something to fix
  skelet fixperms --all    chown all of them without scanning`

// The trees containers write into: the apps' bind mounts, and the storefront workspace's - including the mountpoint a
// storefront's node_modules volume leaves at the root
var trees = []string{"apps", "templates", "packages", "node_modules"}

// present is the trees this checkout has
func present(root string) []string {
	var out []string
	for _, t := range trees {
		if _, err := os.Lstat(filepath.Join(root, t)); err == nil {
			out = append(out, t)
		}
	}
	return out
}

func Run(root string, args []string) int {
	mode := ""
	if len(args) > 0 {
		mode = args[0]
	}
	uid, gid := strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())
	me := uid
	if u, err := user.Current(); err == nil {
		me = u.Username
	}
	switch mode {
	case "", "scan", "--scan":
		fmt.Printf("==> Scanning %v for files not owned by %s…\n", present(root), me)
		strays := scan(root, os.Getuid())
		if len(strays) == 0 {
			fmt.Println("    All clean.")
			return 0
		}
		fmt.Printf("    Found %d entries (showing first 20):\n", len(strays))
		for _, p := range strays[:min(20, len(strays))] {
			fmt.Printf("    %s\n", p)
		}
		fmt.Println()
		fmt.Println("==> Repairing ownership (sudo)…")
		return sudo(root, append(append([]string{"find"}, present(root)...), "-not", "-user", uid, "-exec", "chown", uid+":"+gid, "{}", "+")...)
	case "--all", "all":
		group := gid
		if g, err := user.LookupGroupId(gid); err == nil {
			group = g.Name
		}
		fmt.Printf("==> Blanket chown %v → %s:%s (sudo)…\n", present(root), me, group)
		return sudo(root, append([]string{"chown", "-R", uid + ":" + gid}, present(root)...)...)
	case "-h", "--help", "help":
		fmt.Println(About)
		return 0
	}
	fmt.Fprintf(os.Stderr, "Unknown mode: %s\nRun 'skelet fixperms --help' for usage.\n", mode)
	return 2
}

// scan is find <trees> ! -user: every entry, symlinks by their own owner, unreadable corners skipped
func scan(root string, uid int) []string {
	var strays []string
	for _, t := range present(root) {
		strays = append(strays, scanTree(root, filepath.Join(root, t), uid)...)
	}
	return strays
}

func scanTree(root, top string, uid int) []string {
	var strays []string
	_ = filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != uid {
			rel, _ := filepath.Rel(root, p)
			strays = append(strays, rel)
		}
		return nil
	})
	return strays
}

func sudo(root string, args ...string) int {
	cmd := exec.Command("sudo", args...)
	cmd.Dir = root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := sys.Run(cmd); err != nil {
		return sys.ExitCode(err)
	}
	fmt.Println("    Done.")
	return 0
}
