// Package release is skelet release: build the images of the apps you name and push them to a registry
package release

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const About = `Builds the backends' images and pushes them: REGISTRY=... skelet release core|signals... [--tag T].
Only the ones named ship. The tag defaults to git describe - the release version and the commit - and
PLATFORM to linux/amd64, so a Mac doesn't ship arm64 to amd64 servers. Ends with the line each app's .env
takes. The frontends aren't images: pnpm run build in the app makes what its host serves.`

var argLine = regexp.MustCompile(`(?m)^ARG ([A-Z][A-Z0-9_]*)`)

type image struct {
	name, ctx, dockerfile, envVar, envFile string
}

var images = []image{
	{"core", "apps/backend", "core/Dockerfile.prod", "CORE_IMAGE", "apps/backend/core/.env"},
	{"signals", "apps/backend", "signals/Dockerfile.prod", "SIGNALS_IMAGE", "apps/backend/signals/.env"},
}

func Run(root string, args []string) int {
	tag, names := "", []string{}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--tag" && i+1 < len(args):
			tag = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--tag="):
			tag = strings.TrimPrefix(args[i], "--tag=")
		default:
			names = append(names, args[i])
		}
	}
	known := images
	if len(names) == 0 {
		return usage(known)
	}
	var picked []image
	for _, n := range names {
		i := slices.IndexFunc(known, func(im image) bool { return im.name == n })
		if i < 0 {
			fmt.Fprintf(os.Stderr, "skelet release: no app called %s\n", n)
			return usage(known)
		}
		picked = append(picked, known[i])
	}
	registry := os.Getenv("REGISTRY")
	if registry == "" {
		fmt.Fprintln(os.Stderr, "skelet release: set REGISTRY, e.g. REGISTRY=registry.digitalocean.com/<your-registry>")
		return 1
	}
	if tag == "" {
		out, err := exec.Command("git", "-C", root, "describe", "--tags", "--always").Output()
		if err != nil {
			fmt.Fprintln(os.Stderr, "skelet release: no --tag given and no git commit to name the images after")
			return 1
		}
		tag = strings.TrimSpace(string(out))
	}
	platform := os.Getenv("PLATFORM")
	if platform == "" {
		platform = "linux/amd64"
	}
	dotenv, err := envfile.Pairs(filepath.Join(root, ".env"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet release:", err)
		return 1
	}
	r := run{root: root, registry: registry, tag: tag, platform: platform, dotenv: dotenv}
	for _, im := range picked {
		if code := r.build(im); code != 0 {
			return code
		}
	}
	fmt.Printf("\nPushed tag: %s\nDeploy each with the line its .env takes:\n", tag)
	for _, im := range picked {
		fmt.Printf("  %-26s %s=%s\n", im.envFile, im.envVar, r.ref(im.name))
	}
	return 0
}

func usage(known []image) int {
	var names []string
	for _, im := range known {
		names = append(names, im.name)
	}
	fmt.Fprintln(os.Stderr, "usage: REGISTRY=... skelet release core|signals... [--tag T]")
	fmt.Fprintln(os.Stderr, "  "+strings.Join(names, ", "))
	return 2
}

type run struct {
	root, registry, tag, platform string
	dotenv                        []string
}

func (r run) ref(name string) string { return r.registry + "/skelet-" + name + ":" + r.tag }

func (r run) build(im image) int {
	df := filepath.Join(im.ctx, im.dockerfile)
	raw, err := os.ReadFile(filepath.Join(r.root, df))
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet release:", err)
		return 1
	}
	var names []string
	for _, m := range argLine.FindAllSubmatch(raw, -1) {
		names = append(names, string(m[1]))
	}
	slices.Sort(names)
	args := []string{"build"}
	if r.platform != "" {
		args = append(args, "--platform", r.platform)
	}
	for _, name := range slices.Compact(names) {
		if v := r.setting(name); v != "" {
			args = append(args, "--build-arg", name+"="+v)
		}
	}
	ref := r.ref(im.name)
	if r.registry == "" { // skelet build - for this machine, under the name compose runs
		ref = "skelet-" + im.name + ":" + r.tag
	}
	fmt.Printf("==> build %s  (%s)\n", ref, cmp.Or(r.platform, "this machine's platform"))
	if code := r.docker(append(args, "-f", df, "-t", ref, im.ctx)...); code != 0 {
		return code
	}
	if r.registry == "" {
		return 0
	}
	fmt.Printf("==> push  %s\n", ref)
	return r.docker("push", ref)
}

// Image builds one backend's image for this machine, tagged skelet-<name>:<tag> and pushed nowhere - the name its
// compose service runs when CORE_IMAGE or SIGNALS_IMAGE is unset, with tag prod
func Image(root, name, tag string) int {
	i := slices.IndexFunc(images, func(im image) bool { return im.name == name })
	if i < 0 {
		fmt.Fprintf(os.Stderr, "skelet build: no image called %s\n", name)
		return 2
	}
	dotenv, err := envfile.Pairs(filepath.Join(root, ".env"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet build:", err)
		return 1
	}
	return run{root: root, tag: tag, platform: os.Getenv("PLATFORM"), dotenv: dotenv}.build(images[i])
}

// Images is the names Image takes
func Images() []string {
	out := make([]string, len(images))
	for i, im := range images {
		out[i] = im.name
	}
	return out
}

// setting is a build argument's value: your shell's when set, else the root .env's last line for it
func (r run) setting(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	v := ""
	for _, kv := range r.dotenv {
		if val, ok := strings.CutPrefix(kv, name+"="); ok {
			v = val
		}
	}
	return v
}

func (r run) docker(args ...string) int {
	cmd := exec.Command("docker", args...)
	cmd.Dir = r.root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return sys.ExitCode(sys.Run(cmd))
}
