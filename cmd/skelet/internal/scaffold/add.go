package scaffold

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/pnpm"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const AddAbout = `Makes a storefront in apps/channels/<name> from a template - or from a storefront of your own - with
its own port, volumes and env templates.

  --template  which template to start from (default: nextjs-vetrina, the full one)
  --from      copy one of your own storefronts instead
  --tracked   wire it into the tracked compose files instead of the override

The copy is yours: upstream keeps improving the templates and the packages they're built on, and
since you never edit either, merging a release doesn't collide with your storefront. Every compose
block is copied from the source service rather than templated, so a change to its wiring reaches the
next storefront instead of drifting from a copy kept here. By default the
dev service goes in docker-compose.override.yml, which is git-ignored and auto-merged - so pulling from
upstream skelet never conflicts with your shops (docs/workflow.md). Prod deployment stays yours to wire.

--tracked edits docker-compose{,.prod,.small}.yml instead. That's for adding a storefront to skelet
itself, and it's what makes upstream pulls conflict.

Nothing is written until every generated compose file parses, so a failure leaves the repo untouched.`

const addUsage = "skelet add channel <name> [--template <template> | --from <storefront>] [--tracked]"

const (
	devFile      = "docker-compose.yml"
	prodFile     = "docker-compose.prod.yml"
	smallFile    = "docker-compose.small.yml"
	overrideFile = "docker-compose.override.yml"
)

// Compose service names, directory names and the *_IMAGE variable all derive from a storefront's name, so it's held
// to what all three accept rather than found out three files later
var validName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// Unusable is why name can't be a new storefront's, or "" - setup asks it before writing anything
func Unusable(root, name string) string {
	return unusable(root, storefronts.Known(root), name)
}

func unusable(root string, known []storefronts.Storefront, name string) string {
	if !validName.MatchString(name) {
		return "lowercase letters, digits and inner hyphens only"
	}
	if _, ok := storefronts.Named(known, name); ok || storefronts.Exists(root, name) {
		return "a storefront or template has that name"
	}
	for _, f := range []string{devFile, overrideFile} {
		if declares(read(root, f), name) {
			return f + " has a service by that name"
		}
	}
	return ""
}

var (
	devPort  = regexp.MustCompile(`"[0-9]{4,5}:3000"`)
	hostPort = regexp.MustCompile(`127\.0\.0\.1:[0-9]{4,5}:3000`)
)

type addOptions struct {
	name, src string
	// template says src was named with --template, which only a template answers to
	template, tracked bool
	// quiet leaves the next steps out - setup, making a project's first shop, has its own
	quiet bool
}

func Fork(root, name, template string) int {
	return add(root, addOptions{name: name, src: template, template: template != "", quiet: true})
}

func Add(root string, args []string) int {
	if len(args) == 0 || slices.Contains([]string{"-h", "--help", "help"}, args[0]) {
		fmt.Println(addUsage + "\n\n" + AddAbout)
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
		case "--from", "--template":
			if i+1 >= len(rest) || rest[i+1] == "" {
				fmt.Fprintf(os.Stderr, "%s needs a name\n", rest[i])
				return 2
			}
			if o.src != "" {
				fmt.Fprintln(os.Stderr, "--template and --from each name the source - give one")
				return 2
			}
			o.template = rest[i] == "--template"
			i++
			o.src = rest[i]
		case "--tracked":
			o.tracked = true
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", rest[i])
			fmt.Println(addUsage + "\n\n" + AddAbout)
			return 2
		}
	}
	return add(root, o)
}

func add(root string, o addOptions) int {
	name := o.name
	if name == "" {
		fmt.Fprintln(os.Stderr, "skelet add channel <name> - e.g. skelet add channel eu-shop")
		return 2
	}
	known := storefronts.Known(root)
	if why := unusable(root, known, name); why != "" {
		fmt.Fprintf(os.Stderr, "Can't call it %q - %s\n", name, why)
		return 2
	}
	// The source's name goes into every copied file, so it's held to the same rule before it's used
	if o.src != "" && !validName.MatchString(o.src) {
		fmt.Fprintf(os.Stderr, "Bad source %q - a template or a storefront's name\n", o.src)
		return 2
	}
	from, ok := storefronts.Named(known, o.src)
	if o.src == "" {
		from, ok = defaultSource(root, known)
	}
	switch {
	case !ok:
		fmt.Fprintln(os.Stderr, "No storefront to copy from - pass --template <name> or --from <name>")
		return 2
	case o.template && !from.Template():
		fmt.Fprintf(os.Stderr, "%q is a storefront of yours, not a template - copy it with --from %s\n", from.Name, from.Name)
		return 2
	}
	src := from.Name
	m := newMove(from.Dir, src, name)
	files := portFiles(root)
	dev, small := freePort(3001, files), freePort(13001, files)

	if from.Excluded {
		fmt.Printf("==> Copying %s into apps/channels/%s, from git - it isn't checked out here\n", from.Dir, name)
	} else {
		fmt.Printf("==> Copying %s into apps/channels/%s\n", from.Dir, name)
	}
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
	devBlock := rename(block(read(root, devSrc), src), m, devPort, `"`+strconv.Itoa(dev)+`:3000"`)
	if len(devBlock) == 0 {
		fmt.Fprintf(os.Stderr, "Could not read the %q service out of %s\n", src, devSrc)
		return 1
	}

	writes := map[string]string{}
	var targets []string
	if o.tracked {
		prodBlock := rename(block(read(root, prodFile), src), m, hostPort, "127.0.0.1:"+strconv.Itoa(dev)+":3000")
		smallBlock := rename(block(read(root, smallFile), src), m, hostPort, "127.0.0.1:"+strconv.Itoa(small)+":3000")
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

	// Made aside and moved into place last, so a failure leaves no half-written storefront
	staging, err := sys.Scratch(root, "fork-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet add:", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(staging) }()
	copyFork := fork
	if from.Excluded {
		copyFork = forkFromHead
	}
	copied := filepath.Join(staging, name)
	if err := copyFork(root, m, copied); err != nil {
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
		body := m.applyFile(string(raw))
		if from := smallPortOf(read(root, smallFile), src); from != "" {
			body = whole(body, "localhost:"+from, "localhost:"+strconv.Itoa(small), nil, nil)
		}
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
	dst := filepath.Join(root, filepath.FromSlash(m.dir))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "skelet add:", err)
		return 1
	}
	if err := os.Rename(copied, dst); err != nil {
		fmt.Fprintln(os.Stderr, "skelet add:", err)
		return 1
	}
	// The storefronts are one pnpm workspace with one lockfile, and the new one's dev image installs frozen - so the
	// lockfile learns about it now
	fmt.Printf("==> Adding %s to the workspace lockfile\n", m.dir)
	if err := pnpm.WorkspaceLockfile(root); err != nil {
		fmt.Fprintf(os.Stderr, "    couldn't update pnpm-lock.yaml: %v\n", err)
		fmt.Fprintf(os.Stderr, "    %s is made, but its image won't build until the lockfile has it - skelet pnpm --lockfile finishes it\n", m.dir)
		return 1
	}
	previewOrigin(root, dev)
	if o.quiet {
		fmt.Printf("==> apps/channels/%s is yours, copied from %s - on http://localhost:%d once it runs\n", name, src, dev)
		return 0
	}
	summary(root, o.tracked, src, name, dev, small)
	return 0
}

func smallPortOf(small, service string) string {
	for _, l := range block(small, service) {
		if m := hostPort.FindString(l); m != "" {
			return strings.Split(m, ":")[1]
		}
	}
	return ""
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

func defaultSource(root string, known []storefronts.Storefront) (storefronts.Storefront, bool) {
	if s, ok := storefronts.Default(known); ok {
		return s, true
	}
	dev := read(root, devFile)
	if i := slices.IndexFunc(known, func(s storefronts.Storefront) bool { return declares(dev, s.Name) }); i >= 0 {
		return known[i], true
	}
	if len(known) == 0 {
		return storefronts.Storefront{}, false
	}
	return known[0], true
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
func fork(root string, m move, to string) error {
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", m.srcDir)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return forkTree(root, m, to)
	}
	from := filepath.Join(root, filepath.FromSlash(m.srcDir))
	for f := range strings.SplitSeq(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		rel, ok := strings.CutPrefix(f, m.srcDir+"/")
		if !ok || strayPnpm(rel) {
			continue
		}
		if err := copyOne(filepath.Join(from, filepath.FromSlash(rel)), filepath.Join(to, filepath.FromSlash(rel)), m); err != nil {
			return err
		}
	}
	return nil
}

func forkTree(root string, m move, to string) error {
	from := filepath.Join(root, filepath.FromSlash(m.srcDir))
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		switch {
		case rel == "node_modules" || rel == ".next":
			return filepath.SkipDir
		case d.IsDir() || strayPnpm(rel):
			return nil
		}
		return copyOne(p, filepath.Join(to, rel), m)
	})
}

func copyOne(src, dst string, m move) error {
	info, err := os.Lstat(src)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		link, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(link, dst)
	case !info.Mode().IsRegular():
		return nil
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeForked(dst, raw, info.Mode().Perm(), m)
}

// A workspace member has no pnpm files of its own - a stray pair would make the fork a project apart
func strayPnpm(rel string) bool { return rel == "pnpm-lock.yaml" || rel == "pnpm-workspace.yaml" }

// writeForked writes one of the source's files into the fork, renamed through if it's text
func writeForked(dst string, raw []byte, mode fs.FileMode, m move) error {
	// A binary file - the favicon, the svgs' raster cousins - is copied as it is
	if bytes.IndexByte(raw, 0) < 0 {
		raw = []byte(m.applyFile(string(raw)))
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, raw, mode); err != nil {
		return err
	}
	// Past the umask, the way cp -a keeps it
	return os.Chmod(dst, mode)
}

// forkFromHead is fork for a source the working tree doesn't have - a template setup left out - read as git's HEAD
// holds it. Read whole before any of it is written, so a bad entry can't leave git blocked on a pipe
func forkFromHead(root string, m move, to string) error {
	cmd := exec.Command("git", "archive", "--format=tar", "HEAD", m.srcDir)
	cmd.Dir = root
	archive, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git archive %s: %w", m.srcDir, err)
	}
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, ok := strings.CutPrefix(strings.TrimSuffix(hdr.Name, "/"), m.srcDir+"/")
		if !ok || strayPnpm(rel) {
			continue
		}
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("git archive of %s holds %q, outside it", m.srcDir, hdr.Name)
		}
		dst := filepath.Join(to, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(dst, 0o755)
		case tar.TypeSymlink:
			if err = os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
				err = os.Symlink(hdr.Linkname, dst)
			}
		case tar.TypeReg:
			mode := fs.FileMode(0o644)
			if hdr.Mode&0o111 != 0 {
				mode = 0o755
			}
			var raw []byte
			if raw, err = io.ReadAll(tr); err == nil {
				err = writeForked(dst, raw, mode, m)
			}
		}
		if err != nil {
			return err
		}
	}
}

func previewOrigin(root string, port int) {
	if p, _ := profile.Load(filepath.Join(root, profile.File)); p[profile.TargetKey] != "" && p[profile.TargetKey] != "dev" {
		return
	}
	env, origin := filepath.Join(root, ".env"), "http://localhost:"+strconv.Itoa(port)
	if _, set, err := envfile.Get(env, "VITE_STOREFRONT_ORIGIN"); err != nil || set {
		return
	}
	if err := envfile.Set(env, "VITE_STOREFRONT_ORIGIN", origin); err != nil {
		fmt.Fprintf(os.Stderr, "    couldn't point the dashboard's previews at it: %v\n", err)
		return
	}
	fmt.Printf("==> Dashboard previews open %s now (VITE_STOREFRONT_ORIGIN in .env)\n", origin)
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
		"and STOREFRONT_UPSTREAM in env/prod-caddy.env routes the edge to it. docs/deploying.md covers both.", src)
	if tracked {
		wrote = fmt.Sprintf("docker-compose{,.prod,.small}.yml   service + volumes, dev :%d, small :%d", dev, small)
		deploy = "The edge serves it once STOREFRONT_UPSTREAM in env/prod-caddy.env names it - skelet setup does that for the storefront you deploy."
	}
	// The suggestion adds the new shop to what this clone runs now rather than replacing it - the profile's selection,
	// else every app the tree holds, which is what setup picks when it has no answer on record
	selected := "dashboard"
	p, _ := profile.Load(filepath.Join(root, profile.File))
	if v, ok := p.Get(profile.AppsKey); ok {
		selected = v
	} else {
		for _, n := range storefronts.Names(root) {
			if n != name {
				selected += "," + n
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
