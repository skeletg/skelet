package scaffold

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/pnpm"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
)

const RemoveAbout = `Removes a storefront: its directory, its compose service and volume declarations, and its env
templates - wherever skelet add channel put them, the git-ignored override or the tracked compose files.

  -y          skip the confirmation prompt
  --volumes   also delete its Docker volumes (node_modules and the .next cache)

It prints everything it will delete, with sizes, and asks first: the directory is usually untracked, so git
can't bring it back. Without --volumes the named volumes are left alone, since that's the part you can't
undo and a storefront's .next cache can be many GB.`

var looseName = regexp.MustCompile(`^[a-z0-9-]+$`)

// openTTY is where the confirmation is read: the terminal itself, so a piped stdin can't answer it. A seam for the
// tests, whose children would otherwise inherit the terminal they were started from
var openTTY = func() (io.ReadCloser, error) { return os.Open("/dev/tty") }

func Remove(root string, args []string) int {
	if len(args) == 0 || slices.Contains([]string{"-h", "--help", "help"}, args[0]) {
		fmt.Println("skelet rm channel <name> [-y] [--volumes]\n\n" + RemoveAbout)
		return 0
	}
	if args[0] != "channel" {
		fmt.Fprintf(os.Stderr, "skelet rm: don't know how to remove a %q - only 'channel' so far\n", args[0])
		return 2
	}
	name, force, dropVolumes := "", false, false
	rest := args[1:]
	if len(rest) > 0 {
		name, rest = rest[0], rest[1:]
	}
	for _, a := range rest {
		switch a {
		case "-y", "--yes":
			force = true
		case "--volumes":
			dropVolumes = true
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", a)
			fmt.Println("skelet rm channel <name> [-y] [--volumes]\n\n" + RemoveAbout)
			return 2
		}
	}
	switch {
	case name == "":
		fmt.Fprintln(os.Stderr, "skelet rm channel <name>")
		return 2
	case !looseName.MatchString(name):
		fmt.Fprintf(os.Stderr, "Bad name %q\n", name)
		return 2
	}
	return remove(root, name, force, dropVolumes)
}

func remove(root, name string, force, dropVolumes bool) int {
	if s, ok := storefronts.Find(root, name); ok && s.Template() {
		fmt.Fprintf(os.Stderr, "%q is a template, not one of your storefronts - templates come from upstream, and rm channel leaves them be\n", name)
		return 2
	}
	lib := compose.Load(root)
	// A scaffold lands in one place, but a hand-edited clone might have it in several, so every one is collected
	var composeFiles []string
	for _, f := range []string{overrideFile, devFile, prodFile, smallFile} {
		if declares(read(root, f), name) {
			composeFiles = append(composeFiles, f)
		}
	}
	appDir := ""
	if fi, err := os.Stat(filepath.Join(root, "apps", "channels", name)); err == nil && fi.IsDir() {
		appDir = filepath.Join("apps", "channels", name)
	}
	var envFiles []string
	for _, f := range []string{"prod-" + name + ".env.example", "small-" + name + ".env.example", "prod-" + name + ".env", "small-" + name + ".env"} {
		if _, err := os.Stat(filepath.Join(root, "env", f)); err == nil {
			envFiles = append(envFiles, filepath.Join("env", f))
		}
	}
	volumes := ownVolumes(name)
	out, _ := compose.Command(root, lib.Env(), "ps", "--status", "running", "--format", "{{.Service}}").Output()
	running := slices.Contains(strings.Fields(string(out)), name)

	if len(composeFiles) == 0 && appDir == "" && len(envFiles) == 0 && len(volumes) == 0 {
		fmt.Fprintf(os.Stderr, "Nothing here called %q - no directory, compose service, env file or volume\n", name)
		return 1
	}

	fmt.Printf("Removing storefront %q:\n", name)
	if appDir != "" {
		fmt.Printf("  %-42s %s  (%s)\n", appDir, diskUsage(filepath.Join(root, appDir)), recoverable(root, appDir))
	}
	for _, f := range composeFiles {
		fmt.Printf("  %-42s service + volume declarations\n", f)
	}
	for _, f := range envFiles {
		fmt.Printf("  %-42s\n", f)
	}
	if running {
		fmt.Printf("  %-42s running container, will be stopped\n", name)
	}
	sizes := volumeSizes(volumes)
	for _, v := range volumes {
		size := sizes[v]
		if size == "" {
			size = "?"
		}
		if dropVolumes {
			fmt.Printf("  %-42s %s  DELETED\n", v, size)
		} else {
			fmt.Printf("  %-42s %s  kept (pass --volumes to delete)\n", v, size)
		}
	}

	if !force {
		tty, err := openTTY()
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "refusing to delete without a terminal - re-run with -y to confirm")
			return 1
		}
		fmt.Print("\nContinue? [y/N] ")
		ans, _ := bufio.NewReader(tty).ReadString('\n')
		_ = tty.Close()
		if !slices.Contains([]string{"y", "Y", "yes", "YES"}, strings.TrimSpace(ans)) {
			fmt.Println("aborted")
			return 1
		}
	}

	// Taking the last service out of the override leaves services: with nothing under it, which compose rejects
	// outright - so that case is a file deletion, not an edit
	stripped := map[string]string{}
	dropOverride := false
	for _, f := range composeFiles {
		stripped[f] = strip(read(root, f), name)
		if f == overrideFile && !hasServices(stripped[f]) {
			dropOverride = true
		}
	}
	// The same contract as add - validated before anything is written
	for _, f := range composeFiles {
		if f == overrideFile && dropOverride {
			continue
		}
		if msg, ok := valid(root, f, stripped[f]); !ok {
			fmt.Fprintf(os.Stderr, "Removing it from %s would leave an invalid file - nothing written:\n", f)
			for l := range strings.Lines(msg) {
				fmt.Fprint(os.Stderr, "  "+l)
			}
			return 1
		}
	}

	if running {
		fmt.Printf("==> Stopping %s\n", name)
		_ = compose.Command(root, lib.Env(), "rm", "-sf", name).Run()
	}
	for _, f := range composeFiles {
		if f == overrideFile && dropOverride {
			if err := os.Remove(filepath.Join(root, f)); err != nil {
				return failed(err)
			}
			fmt.Printf("    %s (deleted, it held nothing else)\n", f)
			continue
		}
		if err := writeKeepingMode(filepath.Join(root, f), stripped[f]); err != nil {
			return failed(err)
		}
		fmt.Printf("    %s\n", f)
	}
	for _, f := range envFiles {
		git := exec.Command("git", "rm", "-q", "--cached", f)
		git.Dir = root
		_ = git.Run()
		if err := os.Remove(filepath.Join(root, f)); err != nil {
			return failed(err)
		}
		fmt.Printf("    %s\n", f)
	}
	if appDir != "" {
		if err := os.RemoveAll(filepath.Join(root, appDir)); err != nil {
			return failed(err)
		}
		fmt.Printf("    %s\n", appDir)
	}
	if dropVolumes {
		for _, v := range volumes {
			if exec.Command("docker", "volume", "rm", v).Run() == nil {
				fmt.Printf("    volume %s\n", v)
			} else {
				fmt.Printf("    volume %s (in use, skipped)\n", v)
			}
		}
	}
	if err := deselect(root, name); err != nil {
		return failed(err)
	}
	// The workspace lockfile still lists it - frozen installs don't mind, but a refresh keeps it honest
	if rest := storefronts.Names(root); appDir != "" && len(rest) > 0 {
		if pnpm.Lockfile(root, rest[0]) != nil {
			fmt.Fprintf(os.Stderr, "    pnpm-lock.yaml still lists %s - skelet pnpm %s install --lockfile-only clears it\n", appDir, rest[0])
		}
	}
	fmt.Printf("\nRemoved %q.\n", name)
	if !dropVolumes && len(volumes) > 0 {
		fmt.Printf("Its volumes are still there - skelet rm channel %s --volumes, or docker volume rm.\n", name)
	}
	return 0
}

func failed(err error) int {
	fmt.Fprintln(os.Stderr, "skelet rm:", err)
	return 1
}

// ownVolumes is the storefront's two named volumes, under whatever project created them
func ownVolumes(name string) []string {
	out, _ := exec.Command("docker", "volume", "ls", "--format", "{{.Name}}").Output()
	own := regexp.MustCompile(`_` + regexp.QuoteMeta(name) + `-(node-modules|next-cache)$`)
	var vols []string
	for v := range strings.FieldsSeq(string(out)) {
		if own.MatchString(v) {
			vols = append(vols, v)
		}
	}
	return vols
}

func volumeSizes(vols []string) map[string]string {
	sizes := map[string]string{}
	if len(vols) == 0 {
		return sizes
	}
	out, _ := exec.Command("docker", "system", "df", "-v").Output()
	for l := range strings.Lines(string(out)) {
		f := strings.Fields(l)
		if len(f) > 1 && slices.Contains(vols, f[0]) {
			sizes[f[0]] = f[len(f)-1]
		}
	}
	return sizes
}

// recoverable says whether git could bring the directory back - an untracked one is gone for good, and so is anything
// untracked inside a tracked one, which git diff never sees
func recoverable(root, dir string) string {
	git := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.Output()
		return string(out), err
	}
	if _, err := git("ls-files", "--error-unmatch", dir); err != nil {
		return "untracked - deleting it is NOT recoverable through git"
	}
	if _, err := git("diff", "--quiet", "HEAD", "--", dir); err != nil {
		return "tracked with UNCOMMITTED changes - those changes are not recoverable"
	}
	if extra, _ := git("ls-files", "--others", "--exclude-standard", "--", dir); strings.TrimSpace(extra) != "" {
		return "tracked, with UNTRACKED files in it - those are not recoverable"
	}
	return "committed - recoverable with git checkout"
}

// diskUsage is what du -sh says: the blocks a tree takes, in the unit that keeps the number short
func diskUsage(dir string) string {
	var total int64
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := d.Info(); err == nil {
			if st, ok := info.Sys().(*syscall.Stat_t); ok {
				total += st.Blocks * 512
			} else {
				total += info.Size()
			}
		}
		return nil
	})
	v := float64(total)
	units := []string{"", "K", "M", "G", "T"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d", total)
	}
	if v < 10 {
		return fmt.Sprintf("%.1f%s", math.Ceil(v*10)/10, units[i])
	}
	return fmt.Sprintf("%.0f%s", math.Ceil(v), units[i])
}

// deselect takes the storefront out of .skelet-apps - left in, compose warns about a profile that matches no service
// on every command. Comma-separated fields, not a substring: a hyphen is a word boundary, so pattern-matching shop
// out of eu-shop,dashboard would leave eu-dashboard
func deselect(root, name string) error {
	path := filepath.Join(root, ".skelet-apps")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	was := strings.Join(strings.Fields(string(raw)), "")
	var kept []string
	for a := range strings.SplitSeq(was, ",") {
		if a != name && a != "" {
			kept = append(kept, a)
		}
	}
	if strings.Join(kept, ",") == was {
		return nil
	}
	if err := os.WriteFile(path, []byte(strings.Join(kept, ",")+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("    .skelet-apps (dropped %s)\n", name)
	return nil
}
