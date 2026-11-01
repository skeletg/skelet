// Package release is skelet release: build the production images and push them to a registry
package release

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const About = `Builds the production images and pushes them: REGISTRY=... skelet release [tag]. The tag defaults to
the commit's short sha, and PLATFORM to linux/amd64, so a Mac doesn't ship arm64 to amd64 servers.
A build argument a Dockerfile declares (UPLOAD_ORIGINS, the preview origins) comes from your shell,
else the root .env. Every frontend this clone runs ships - the app selection skelet setup made, or
without one every storefront with a Dockerfile.prod, templates included - so a fork ships its own shop
without editing anything. Ends with the exports docker-compose.prod.yml reads.`

var argLine = regexp.MustCompile(`(?m)^ARG ([A-Z][A-Z0-9_]*)`)

func Run(root string, args []string) int {
	registry := os.Getenv("REGISTRY")
	if registry == "" {
		fmt.Fprintln(os.Stderr, "skelet release: set REGISTRY, e.g. REGISTRY=registry.digitalocean.com/<your-registry>")
		return 1
	}
	tag := ""
	if len(args) > 0 {
		tag = args[0]
	} else {
		out, err := exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD").Output()
		if err != nil {
			fmt.Fprintln(os.Stderr, "skelet release: no tag given and no git commit to name the images after")
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

	prof, err := profile.Load(filepath.Join(root, profile.File))
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet release:", err)
		return 1
	}
	selected, chosen := prof.Apps()
	ships := func(app string) bool { return !chosen || slices.Contains(selected, app) }

	// Both Go images build from apps/backend as a whole - the modules replace each other by relative path
	images := [][3]string{{"app", "apps/backend", "core/Dockerfile.prod"}, {"signals", "apps/backend", "signals/Dockerfile.prod"}}
	services := []string{"app", "signals"}
	if ships("dashboard") {
		images = append(images, [3]string{"dashboard", "apps/dashboard", "Dockerfile.prod"})
		services = append(services, "dashboard")
	}
	var exports []string
	// A storefront builds from the repo root, the workspace it shares packages through
	for _, sf := range storefronts.List(root) {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(sf.Dir), "Dockerfile.prod")); err != nil {
			continue
		}
		if !ships(sf.Name) {
			fmt.Printf("==> skipping %s - not in this clone's app selection (skelet setup --apps ...)\n", sf.Dir)
			continue
		}
		name := sf.Name
		images = append(images, [3]string{name, ".", sf.Dir + "/Dockerfile.prod"})
		// The env var is the directory upper-cased, which is what the compose files read
		exports = append(exports, fmt.Sprintf("  export %s_IMAGE=%s", strings.ToUpper(strings.ReplaceAll(name, "-", "_")), r.ref(name)))
	}
	for _, img := range images {
		if code := r.build(img[0], img[1], img[2]); code != 0 {
			return code
		}
	}
	fmt.Printf("\nPushed tag: %s\nDeploy with (docker-compose.prod.yml reads these):\n", tag)
	for _, svc := range services {
		fmt.Printf("  export %s_IMAGE=%s\n", strings.ToUpper(svc), r.ref(svc))
	}
	for _, e := range exports {
		fmt.Println(e)
	}
	fmt.Println("  docker compose -f docker-compose.prod.yml up -d")
	return 0
}

type run struct {
	root, registry, tag, platform string
	dotenv                        []string
}

func (r run) ref(svc string) string { return r.registry + "/skelet-" + svc + ":" + r.tag }

func (r run) build(svc, ctx, dockerfile string) int {
	df := filepath.Join(ctx, dockerfile)
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
	args := []string{"build", "--platform", r.platform}
	for _, name := range slices.Compact(names) {
		if v := r.setting(name); v != "" {
			args = append(args, "--build-arg", name+"="+v)
		}
	}
	ref := r.ref(svc)
	fmt.Printf("==> build %s  (%s)\n", ref, r.platform)
	if code := r.docker(append(args, "-f", df, "-t", ref, ctx)...); code != 0 {
		return code
	}
	fmt.Printf("==> push  %s\n", ref)
	return r.docker("push", ref)
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
