package pluginset

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A module plugin's source lives read-only in the module cache, at a path that contains its version and
// differs per machine. gqlgen's schema list has to be a stable relative path committed to the repo, so
// what a module plugin ships beside its Go code gets copied into StageDir first
//
// Copying rather than symlinking or writing absolute paths: the cache entry is immutable, so the copy is
// reproducible, and a relative path is the only thing that works the same in CI, in Docker and on a
// developer's machine
//
// Only GraphQL SDL today. The frontend halves want the same treatment and would slot in here

// Stage refreshes StageDir to hold exactly the enabled module plugins' SDL, and nothing else - a plugin
// disabled or removed since the last run has its staged copy swept, so a stale directory can never keep
// feeding schema into codegen
func Stage(root string) error {
	set, err := Load(root)
	if err != nil {
		return err
	}
	stageRoot := filepath.Join(root, StageDir)
	// Rebuilt from scratch every run - reconciling would mean tracking what a previous version shipped
	if err := os.RemoveAll(stageRoot); err != nil {
		return fmt.Errorf("clear %s: %w", StageDir, err)
	}
	slugs := set.ModuleSlugs()
	if len(slugs) == 0 {
		return nil
	}
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		return err
	}
	for _, slug := range slugs {
		modDir, err := moduleDir(root, set.Plugins[slug].Module)
		if err != nil {
			return fmt.Errorf("plugin %s: %w", slug, err)
		}
		for _, surface := range []string{"admin", "front"} {
			src := filepath.Join(modDir, "graphql", surface)
			if _, err := os.Stat(src); err != nil {
				continue // this plugin extends that surface with nothing
			}
			dst := filepath.Join(stageRoot, slug, "graphql", surface)
			if err := copyDirSQL(src, dst); err != nil {
				return fmt.Errorf("plugin %s: stage %s SDL: %w", slug, surface, err)
			}
		}
	}
	return nil
}

// Asks the go tool where a module resolved to, which honours replace directives and workspaces - so a
// plugin being developed from a local checkout stages from there, and a published one from the cache
func moduleDir(root, module string) (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", module)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// go list puts the useful part on stderr - "module not in go.mod", "unknown revision" - so
		// surface it, or the caller gets a bare exit status for a very fixable problem
		var stderr string
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return "", fmt.Errorf("go list -m %s: %w: %s", module, err, stderr)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", fmt.Errorf("module %s has no directory - is it required in go.mod?", module)
	}
	return dir, nil
}

// Only .graphqls, so nothing else a plugin happens to keep under graphql/ lands in the schema glob
func copyDirSQL(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	made := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".graphqls") {
			continue
		}
		if !made {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			made = true
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// 0644 explicitly: the module cache is read-only (0444), and copying the mode through would make the
// staged tree undeletable on the next run
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
