package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Git fetches releases from a git remote by their tags
type Git struct {
	URL string
	// Out takes git's own progress; os.Stderr when nil
	Out io.Writer
}

func (g Git) Latest(ctx context.Context) (string, error) {
	var out bytes.Buffer
	if err := g.git(ctx, &out, "ls-remote", "--tags", "--refs", g.URL, "v*"); err != nil {
		return "", fmt.Errorf("listing the releases at %s: %w", g.URL, err)
	}
	var tags []string
	for line := range strings.Lines(out.String()) {
		if _, ref, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
			tags = append(tags, strings.TrimPrefix(ref, "refs/tags/"))
		}
	}
	tag, ok := newest(tags)
	if !ok {
		return "", fmt.Errorf("%s has no release tags", g.URL)
	}
	return tag, nil
}

func (g Git) Create(ctx context.Context, version, dir string) error {
	// blob:none brings the history without old file contents, which arrive only if something asks for them
	if err := g.git(ctx, nil, "-c", "advice.detachedHead=false", "clone", "--filter=blob:none", "--origin", "upstream",
		"--branch", version, "--", g.URL, dir); err != nil {
		return fmt.Errorf("fetching %s from %s: %w", version, g.URL, err)
	}
	// The tag leaves HEAD detached - main is where your own commits go, starting at the release
	if err := g.git(ctx, nil, "-C", dir, "checkout", "-q", "-b", "main"); err != nil {
		return fmt.Errorf("starting main at %s: %w", version, err)
	}
	return nil
}

func (g Git) git(ctx context.Context, stdout io.Writer, args ...string) error {
	log := g.Out
	if log == nil {
		log = os.Stderr
	}
	if stdout == nil {
		stdout = log
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout, cmd.Stderr = stdout, log
	err := cmd.Run()
	if errors.Is(err, exec.ErrNotFound) {
		return errors.New("git is needed for this and isn't on the PATH")
	}
	return err
}
