package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/skeletg/skelet/apps/skelet/internal/checkout"
)

// Options is one skelet new <dir>
type Options struct {
	Dir string
	// Version is the release to start from; empty means the binary's own, else the newest
	Version string
	Repo    string
	// Force allows a project inside another one
	Force bool
	// Setup is every other argument, handed to the new project's setup untouched
	Setup []string
}

var ErrHelp = errors.New("help asked for")

// ParseArgs reads the arguments after new. The directory comes first, so a setup flag's value is never mistaken for it
func ParseArgs(args []string) (Options, error) {
	o := Options{Repo: Upstream}
	if slices.Contains(args, "-h") || slices.Contains(args, "--help") {
		return o, ErrHelp
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return o, errors.New("the directory comes first: skelet new <dir> [flags]")
	}
	o.Dir = args[0]
	for i := 1; i < len(args); i++ {
		name, val, inline := strings.Cut(args[i], "=")
		switch name {
		case "--version", "--repo":
			if !inline {
				if i+1 == len(args) {
					return o, fmt.Errorf("%s needs a value", name)
				}
				i++
				val = args[i]
			}
			if val == "" || strings.HasPrefix(val, "-") {
				return o, fmt.Errorf("%s needs a value, not %q", name, val)
			}
			if name == "--version" {
				o.Version = val
			} else {
				o.Repo = val
			}
		case "--force":
			o.Force = true
		default:
			o.Setup = append(o.Setup, args[i])
		}
	}
	return o, nil
}

// Create fetches the release into o.Dir and returns the project's absolute path and the tag it holds. own is the
// running binary's version, the release a project starts from when --version doesn't say
func Create(ctx context.Context, src Source, o Options, own string, log io.Writer) (string, string, error) {
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return "", "", err
	}
	if root, ok := checkout.Find(filepath.Dir(dir)); ok && !o.Force {
		return "", "", fmt.Errorf("%s would be inside the Skelet project at %s - pick a directory outside it, or pass --force", dir, root)
	}
	entries, err := os.ReadDir(dir)
	switch {
	case err == nil && len(entries) > 0:
		return "", "", fmt.Errorf("%s already exists and isn't empty", dir)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return "", "", err
	}
	tag := o.Version
	if tag == "" && strings.HasPrefix(own, "v") {
		tag = own
	}
	if tag == "" {
		if tag, err = src.Latest(ctx); err != nil {
			return "", "", err
		}
	}
	fmt.Fprintf(log, "==> fetching Skelet %s into %s\n", tag, dir)
	if err := src.Create(ctx, tag, dir); err != nil {
		return "", "", err
	}
	return dir, tag, nil
}
