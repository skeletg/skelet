package scaffold

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
)

const AddAbout = `Forks a storefront into apps/channels/<name> with its own port, volumes and env templates.

  --from      which storefront to copy (default: the one docker-compose.yml declares)
  --tracked   wire it into the tracked compose files instead of the override

Every compose block is copied from the source service rather than templated, so a change to the
demo's wiring reaches the next storefront instead of drifting from a copy kept here. By default the
dev service goes in docker-compose.override.yml, which is git-ignored and auto-merged - so pulling from
upstream skelet never conflicts with your shops (docs/workflow.md). Prod deployment stays yours to wire.

--tracked edits docker-compose{,.prod,.small}.yml instead. That's for adding a storefront to skelet
itself, and it's what makes upstream pulls conflict.

Nothing is written until every generated compose file parses, so a failure leaves the repo untouched.`

const (
	devFile      = "docker-compose.yml"
	prodFile     = "docker-compose.prod.yml"
	smallFile    = "docker-compose.small.yml"
	overrideFile = "docker-compose.override.yml"
)

// Compose service names, directory names and the *_IMAGE variable all derive from a storefront's name, so it's held
// to what all three accept rather than found out three files later
var validName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

var (
	devPort   = regexp.MustCompile(`"[0-9]{4,5}:3000"`)
	prodPort  = regexp.MustCompile(`127\.0\.0\.1:3000:3000`)
	smallPort = regexp.MustCompile(`127\.0\.0\.1:13000:3000`)
)

type addOptions struct {
	name, src string
	tracked   bool
}

func Add(root string, args []string) int {
	if len(args) == 0 || slices.Contains([]string{"-h", "--help", "help"}, args[0]) {
		fmt.Println("skelet add channel <name> [--from <storefront>] [--tracked]\n\n" + AddAbout)
		return 0
	}
	if args[0] != "channel" {
		fmt.Fprintf(os.Stderr, "skelet add: don't know how to make a %q - only 'channel' so far\n", args[0])
		return 2
	}
	o := addOptions{}
	rest := args[1:]
	if len(rest) > 0 {
		o.name, rest = rest[0], rest[1:]
	}
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--from":
			if i+1 >= len(rest) || rest[i+1] == "" {
				fmt.Fprintln(os.Stderr, "--from needs a storefront name")
				return 2
			}
			i++
			o.src = rest[i]
		case "--tracked":
			o.tracked = true
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", rest[i])
			fmt.Println("skelet add channel <name> [--from <storefront>] [--tracked]\n\n" + AddAbout)
			return 2
		}
	}
	return add(root, o)
}

func add(root string, o addOptions) int {
	name := o.name
	switch {
	case name == "":
		fmt.Fprintln(os.Stderr, "skelet add channel <name> - e.g. skelet add channel eu-shop")
		return 2
	case !validName.MatchString(name):
		fmt.Fprintf(os.Stderr, "Bad name %q - lowercase letters, digits and inner hyphens only\n", name)
		return 2
	}
	if _, err := os.Lstat(filepath.Join(root, "apps", "channels", name)); err == nil {
		fmt.Fprintf(os.Stderr, "apps/channels/%s already exists\n", name)
		return 2
	}
	for _, f := range []string{devFile, overrideFile} {
		if declares(read(root, f), name) {
			fmt.Fprintf(os.Stderr, "%s already has a %q service\n", f, name)
			return 2
		}
	}
	// The source's name goes into every copied file, so it's held to the same rule before it's used
	if o.src != "" && !validName.MatchString(o.src) {
		fmt.Fprintf(os.Stderr, "Bad --from %q - a storefront under apps/channels/\n", o.src)
		return 2
	}
	src := o.src
	if src == "" {
		src = defaultSource(root)
	}
	if fi, err := os.Stat(filepath.Join(root, "apps", "channels", src)); src == "" || err != nil || !fi.IsDir() {
		fmt.Fprintln(os.Stderr, "No storefront to copy from - pass --from <name>")
		return 2
	}
	files := portFiles(root)
	dev, small := freePort(3001, files), freePort(13001, files)

	fmt.Printf("==> Forking %s into apps/channels/%s\n", src, name)
	devSrc := ""
	for _, f := range []string{devFile, overrideFile} {
		if declares(read(root, f), src) {
			devSrc = f
			break
		}
	}
	if devSrc == "" {
		fmt.Fprintf(os.Stderr, "No %q service in %s or %s - can't copy its wiring\n", src, devFile, overrideFile)
		return 1
	}
	// Its published port is whatever that file gave it, so it's rewritten by matching the container side
	devBlock := rename(block(read(root, devSrc), src), src, name, devPort, `"`+strconv.Itoa(dev)+`:3000"`)
	if len(devBlock) == 0 {
		fmt.Fprintf(os.Stderr, "Could not read the %q service out of %s\n", src, devSrc)
		return 1
	}

	writes := map[string]string{}
	var targets []string
	if o.tracked {
		prodBlock := rename(block(read(root, prodFile), src), src, name, prodPort, "127.0.0.1:"+strconv.Itoa(dev)+":3000")
		smallBlock := rename(block(read(root, smallFile), src), src, name, smallPort, "127.0.0.1:"+strconv.Itoa(small)+":3000")
		for i, b := range [][]string{prodBlock, smallBlock} {
			if len(b) == 0 {
				fmt.Fprintf(os.Stderr, "Could not find a %q service in the %s compose file\n", src, []string{"prod", "small"}[i])
				return 1
			}
		}
		// The dev file also declares the two named volumes that shadow node_modules and .next
		writes[devFile] = addVolumes(spliceAfter(read(root, devFile), src, devBlock), src, name)
		writes[prodFile] = spliceAfter(read(root, prodFile), src, prodBlock)
		writes[smallFile] = spliceAfter(read(root, smallFile), src, smallBlock)
		targets = []string{devFile, prodFile, smallFile}
	} else {
		var existing *string
		if raw, err := os.ReadFile(filepath.Join(root, overrideFile)); err == nil {
			s := string(raw)
			existing = &s
		}
		text, ok := override(existing, devBlock, anchors(devBlock, read(root, devFile), deref(existing)), name)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s has no services: section to add %q to - nothing written\n", overrideFile, name)
			return 1
		}
		writes[overrideFile] = text
		targets = []string{overrideFile}
	}

	fmt.Println("==> Checking the generated compose files parse")
	for _, f := range targets {
		if msg, ok := valid(root, f, writes[f]); !ok {
			fmt.Fprintf(os.Stderr, "    %s would be invalid - nothing written:\n", f)
			for l := range strings.Lines(msg) {
				fmt.Fprint(os.Stderr, "      "+l)
			}
			return 1
		}
		fmt.Printf("    %s ok\n", f)
	}

	// Everything validated, so start writing
	if err := fork(root, src, name); err != nil {
		fmt.Fprintln(os.Stderr, "skelet add:", err)
		return 1
	}
	for _, f := range targets {
		if err := writeKeepingMode(filepath.Join(root, f), writes[f]); err != nil {
			fmt.Fprintln(os.Stderr, "skelet add:", err)
			return 1
		}
	}
	for _, tier := range []string{"prod", "small"} {
		tpl := filepath.Join("env", tier+"-"+src+".env.example")
		out := filepath.Join("env", tier+"-"+name+".env.example")
		raw, err := os.ReadFile(filepath.Join(root, tpl))
		if err != nil {
			continue
		}
		body := strings.ReplaceAll(strings.ReplaceAll(string(raw), src, name), "localhost:13000", "localhost:"+strconv.Itoa(small))
		if err := os.WriteFile(filepath.Join(root, out), []byte(body), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "skelet add:", err)
			return 1
		}
		if o.tracked {
			// Staged, not just written - skelet check demands a committed .example for every compose env_file, and the
			// tracked prod block just added one, so an unstaged template would block the next commit
			git := exec.Command("git", "add", out)
			git.Dir = root
			_ = git.Run()
			fmt.Printf("    %s (staged)\n", out)
		} else {
			fmt.Printf("    %s\n", out)
		}
	}
	summary(root, o.tracked, src, name, dev, small)
	return 0
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func read(root, file string) string {
	raw, _ := os.ReadFile(filepath.Join(root, file))
	return string(raw)
}

// defaultSource prefers a storefront the tracked compose file declares - that's the upstream template, and whatever
// sorts first could be last week's experiment
func defaultSource(root string) string {
	manifests, _ := filepath.Glob(filepath.Join(root, "apps", "channels", "*", "package.json"))
	first := ""
	dev := read(root, devFile)
	for _, m := range manifests {
		c := filepath.Base(filepath.Dir(m))
		if declares(dev, c) {
			return c
		}
		if first == "" {
			first = c
		}
	}
	return first
}

// portFiles is every file a host port can be named in - the override too, where earlier storefronts live, and the env
// templates, where an override-only shop's small-tier port appears
func portFiles(root string) string {
	var b strings.Builder
	for _, f := range []string{devFile, prodFile, smallFile, overrideFile} {
		b.WriteString(read(root, f))
	}
	for _, pat := range []string{"env/small-*.env.example", "env/prod-*.env.example"} {
		matches, _ := filepath.Glob(filepath.Join(root, pat))
		for _, m := range matches {
			raw, _ := os.ReadFile(m)
			b.Write(raw)
		}
	}
	return b.String()
}

// freePort is the first host port at or above base that no file names, as a compose mapping ("3001:3000") or a URL
// (localhost:13001) - so deleting a storefront leaves no hole for the next one to collide with
func freePort(base int, files string) int {
	for p := base; ; p++ {
		if !regexp.MustCompile(`(?m)["':]` + strconv.Itoa(p) + `([^0-9]|$)`).MatchString(files) {
			return p
		}
	}
}

// valid runs docker compose config over a would-be file. The override is only valid merged with the base, the way
// compose reads it. --no-interpolate, since the prod files demand secrets a dev clone has no reason to hold - the
// splice is what's under test, not whether this machine could deploy
func valid(root, file, content string) (string, bool) {
	dir, err := os.MkdirTemp("", "skelet-compose-")
	if err != nil {
		return err.Error(), false
	}
	defer func() { _ = os.RemoveAll(dir) }()
	tmp := filepath.Join(dir, file)
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return err.Error(), false
	}
	args := []string{"--project-directory", "."}
	if file == overrideFile {
		args = append(args, "-f", devFile)
	}
	args = append(args, "-f", tmp, "config", "-q", "--no-interpolate")
	cmd := compose.Command(root, compose.Load(root).Env(), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return stderr.String(), cmd.Run() == nil
}

// fork copies the source storefront and renames it through every text file. Its node_modules and .next stay behind -
// installs and build output, regenerated by the new one's own container
func fork(root, src, name string) error {
	from, to := filepath.Join(root, "apps", "channels", src), filepath.Join(root, "apps", "channels", name)
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		if rel == "node_modules" || rel == ".next" {
			return filepath.SkipDir
		}
		dst := filepath.Join(to, rel)
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, dst)
		case info.IsDir():
			return os.MkdirAll(dst, info.Mode().Perm())
		case !info.Mode().IsRegular():
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		// A binary file - the favicon, the svgs' raster cousins - is copied as it is
		if bytes.IndexByte(raw, 0) < 0 {
			raw = bytes.ReplaceAll(raw, []byte(src), []byte(name))
		}
		if err := os.WriteFile(dst, raw, info.Mode().Perm()); err != nil {
			return err
		}
		// Past the umask, the way cp -a keeps it
		return os.Chmod(dst, info.Mode().Perm())
	})
}

// writeKeepingMode replaces a file's content in place, so a compose file keeps its mode
func writeKeepingMode(path, content string) error {
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, []byte(content), mode)
}

func summary(root string, tracked bool, src, name string, dev, small int) {
	wrote := fmt.Sprintf("%s       service + volumes on :%d (git-ignored)", overrideFile, dev)
	deploy := fmt.Sprintf("Prod is yours to wire: docker-compose.prod.yml's %q block is the shape,\n"+
		"and deploy/caddy/Caddyfile has its route. docs/deploying.md covers both.", src)
	if tracked {
		wrote = fmt.Sprintf("docker-compose{,.prod,.small}.yml   service + volumes, dev :%d, small :%d", dev, small)
		deploy = fmt.Sprintf("Deploying it also needs a Caddy route - deploy/caddy/Caddyfile has %s's.", src)
	}
	// The suggestion adds the new shop to what this clone runs now rather than replacing it - the profile's selection,
	// else every app the tree holds, which is what setup picks when it has no answer on record
	selected := "dashboard"
	p, _ := profile.Load(filepath.Join(root, profile.File))
	if v, ok := p.Get("SKELET_APPS_SELECTED"); ok {
		selected = v
	} else {
		dirs, _ := filepath.Glob(filepath.Join(root, "apps", "channels", "*"))
		for _, d := range dirs {
			if fi, err := os.Stat(d); err == nil && fi.IsDir() && filepath.Base(d) != name {
				selected += "," + filepath.Base(d)
			}
		}
	}
	apps := name
	if selected != "" {
		apps = selected + "," + name
	}
	fmt.Printf(`
Wrote:
  apps/channels/%[1]s   forked from %[2]s
  %[3]s
  env/{prod,small}-%[1]s.env.example

Next:
  1. Create the channel in admin > channels, and put its uuid in
     STOREFRONT_CHANNEL_ID (or map hosts to channels in
     STOREFRONT_CHANNEL_HOSTS to serve several from one deployment). The dev
     block inherited %[2]s's default uuid, so change it there too.
  2. skelet setup --apps %[4]s
  3. skelet up, then http://localhost:%[5]d

%[6]s
`, name, src, wrote, apps, dev, deploy)
}
