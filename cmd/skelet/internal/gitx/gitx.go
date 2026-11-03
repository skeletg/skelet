// Package gitx runs the few git commands setup needs against the checkout
package gitx

import (
	"errors"
	"os"
	"os/exec"
	"path"
	"strings"
)

func IsWorkTree(root string) bool {
	out, err := output(root, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// TreeDirs lists the directories under dir in HEAD, by name - the ones sparse-checkout took out of the tree included
func TreeDirs(root, dir string) []string {
	out, err := output(root, "ls-tree", "-d", "--name-only", "HEAD", strings.TrimSuffix(dir, "/")+"/")
	if err != nil || out == "" {
		return nil
	}
	var names []string
	for l := range strings.SplitSeq(out, "\n") {
		names = append(names, path.Base(l))
	}
	return names
}

// Show is a file as HEAD has it, for an app sparse-checkout removed from the working tree
func Show(root, file string) ([]byte, error) {
	cmd := exec.Command("git", "show", "HEAD:"+file)
	cmd.Dir = root
	return cmd.Output()
}

func Indexed(root, file string) ([]byte, error) {
	cmd := exec.Command("git", "show", ":"+file)
	cmd.Dir = root
	return cmd.Output()
}

func SkipWorktree(root string, pathspecs ...string) []string {
	out, err := output(root, append([]string{"ls-files", "-t", "--"}, pathspecs...)...)
	if err != nil {
		return nil
	}
	var files []string
	for l := range strings.SplitSeq(out, "\n") {
		if f, ok := strings.CutPrefix(l, "S "); ok {
			files = append(files, f)
		}
	}
	return files
}

func HasTag(root, tag string) bool {
	_, err := output(root, "rev-parse", "-q", "--verify", "refs/tags/"+tag)
	return err == nil
}

// ChangedSince is whether any of paths in the working tree differs from tag - an error when git can't tell
func ChangedSince(root, tag string, paths ...string) (bool, error) {
	cmd := exec.Command("git", append([]string{"diff", "--quiet", tag, "--"}, paths...)...)
	cmd.Dir = root
	err := cmd.Run()
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, err
}

func SparseOn(root string) bool {
	out, _ := output(root, "config", "core.sparseCheckout")
	return out == "true"
}

// SparseSet keeps only what patterns allow in the working tree; history keeps everything
func SparseSet(root string, patterns []string) error {
	return run(root, append([]string{"sparse-checkout", "set", "--no-cone"}, patterns...)...)
}

func SparseDisable(root string) error { return run(root, "sparse-checkout", "disable") }

func output(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func run(root string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
