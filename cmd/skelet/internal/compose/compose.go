// Package compose gives the CLI's own docker compose calls what bin/_lib gives the scripts: the profiles that follow
// .skelet-apps, .skelet-tools and SIGNALS_ENABLED, the locale, and the uid containers run as. None of it reaches a
// script's environment - _lib keeps an inherited COMPOSE_PROFILES, so a leaked one would override what the script
// works out for itself (--lean included)
package compose

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

// Lib is what bin/_lib works out
type Lib struct {
	root     string
	Profiles string
	// Apps is what runs in Docker, Tools the dev tools in effect - SKELET_TOOLS when set, even empty, else .skelet-tools.
	// Picked is .skelet-tools whatever is in effect, so a --lean run doesn't unwire the tools it leaves out
	Apps, Tools, Picked string
}

var signalsOff = regexp.MustCompile(`(?im)^[ \t]*SIGNALS_ENABLED[ \t]*=[ \t]*(false|0|f|no|off)[ \t]*$`)

// Load works the profiles out. Set-but-empty and unset differ: SKELET_SIDECARS= is --lean, while an unset one means
// the search sidecar; an inherited COMPOSE_PROFILES, even an empty one, wins over all of it
func Load(root string) Lib { return load(root, false) }

// LoadLean is skelet up --lean: no sidecars and no dev tools, while the apps still start
func LoadLean(root string) Lib { return load(root, true) }

func load(root string, lean bool) Lib {
	l := Lib{root: root, Apps: squeeze(root, ".skelet-apps"), Picked: squeeze(root, ".skelet-tools")}
	l.Tools = l.Picked
	if v, ok := os.LookupEnv("SKELET_TOOLS"); ok {
		l.Tools = v
	}
	sidecars, ok := os.LookupEnv("SKELET_SIDECARS")
	if !ok {
		sidecars = "search"
	}
	if lean {
		sidecars, l.Tools = "", ""
	}
	var p []string
	for _, v := range []string{sidecars, l.Apps, l.Tools} {
		if v != "" {
			p = append(p, v)
		}
	}
	// The Signals process follows core's switch, and unset means on - as the backend reads it. A sidecar, so --lean
	// drops it with the others
	if goEnv, _ := os.ReadFile(filepath.Join(root, "env", "go.env")); sidecars != "" && !signalsOff.Match(goEnv) {
		p = append(p, "signals")
	}
	l.Profiles = strings.Join(p, ",")
	if v, ok := os.LookupEnv("COMPOSE_PROFILES"); ok {
		l.Profiles = v
	}
	return l
}

// squeeze is a selection file with every space and newline taken out, or empty when there's no file
func squeeze(root, name string) string {
	raw, _ := os.ReadFile(filepath.Join(root, name))
	return strings.Join(strings.Fields(string(raw)), "")
}

// Env is the environment for a compose command that creates no containers
func (l Lib) Env() []string {
	return set(sys.Env(), "COMPOSE_PROFILES="+l.Profiles, "LC_ALL=C.UTF-8", "LANG=C.UTF-8")
}

// CreateEnv adds the uid and gid compose's user: entries and the frontend build args read - only worked out here,
// since asking a rootless engine costs a docker info
func (l Lib) CreateEnv() []string {
	uid, gid, ok := IDs()
	if !ok {
		return l.Env()
	}
	return set(l.Env(), "SKELET_UID="+uid, "SKELET_GID="+gid)
}

// IDs is the uid and gid containers run as: yours, or 0 under a rootless engine, where container root already is you.
// ok is false when SKELET_UID is set already - it's kept as given, and SKELET_GID with it, even unset
func IDs() (uid, gid string, ok bool) {
	if os.Getenv("SKELET_UID") != "" {
		return "", "", false
	}
	if out, err := exec.Command("docker", "info", "-f", "{{.SecurityOptions}}").Output(); err == nil && strings.Contains(string(out), "rootless") {
		return "0", "0", true
	}
	return strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid()), true
}

// UserFlags pin the locale in a container - exec doesn't reliably inherit it on Docker Desktop - and with
// SKELET_NO_USER_MAP set run as root, for the odd apt-get
func UserFlags() []string {
	flags := []string{"-e", "LC_ALL=C.UTF-8", "-e", "LANG=C.UTF-8"}
	if os.Getenv("SKELET_NO_USER_MAP") != "" {
		flags = append([]string{"-u", "0:0"}, flags...)
	}
	return flags
}

// RunAs is the -u a plain docker run needs to write files as you - a compose service reads SKELET_UID, a docker run
// has nothing to read it from. Nothing under SKELET_NO_USER_MAP, whose -u 0:0 UserFlags carries
func RunAs() []string {
	if os.Getenv("SKELET_NO_USER_MAP") != "" {
		return nil
	}
	uid, gid, ok := IDs()
	if !ok {
		uid, gid = os.Getenv("SKELET_UID"), os.Getenv("SKELET_GID")
	}
	if gid == "" {
		return []string{"-u", uid}
	}
	return []string{"-u", uid + ":" + gid}
}

// Call is docker compose with args, from the checkout with env - for a verb to become, or to run and wait on
func Call(root string, env []string, args ...string) sys.Call {
	return sys.Call{Dir: root, Path: sys.Lookup("docker"), Argv: append([]string{"docker", "compose"}, args...), Env: env}
}

// Command is docker compose run from the checkout with env
func Command(root string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir, cmd.Env = root, env
	return cmd
}

// Running reports whether service has a running container in this project - an exact name, where the scripts' grep
// for skelet-<service> also matched longer names and ignored COMPOSE_PROJECT_NAME
func (l Lib) Running(service string) bool {
	out, _ := Command(l.root, l.Env(), "ps", "--services", "--status", "running").Output()
	return slices.Contains(strings.Fields(string(out)), service)
}

// Target is where ExecOrRun runs a command
type Target struct {
	Service string
	// TTY asks for -it when stdin is a terminal, so piped output stays clean
	TTY bool
	// NoDeps keeps a one-off run from starting the service's dependencies
	NoDeps bool
	// Flags go before the service name: -w, -e, UserFlags
	Flags []string
}

// ExecOrRun is the compose arguments that run cmd in the target's container - exec when it's running, else a one-off
// run removed afterwards. creates says which, since only a run needs CreateEnv
func (l Lib) ExecOrRun(t Target, cmd ...string) (args []string, creates bool) {
	if l.Running(t.Service) {
		args = []string{"exec"}
	} else {
		args, creates = []string{"run", "--rm"}, true
		if t.NoDeps {
			args = append(args, "--no-deps")
		}
	}
	if t.TTY && sys.IsTerminal(os.Stdin.Fd()) {
		args = append(args, "-it")
	}
	args = append(append(args, t.Flags...), t.Service)
	return append(args, cmd...), creates
}

// set replaces or adds each KEY=value in env
func set(env []string, kvs ...string) []string {
	out := slices.Clone(env)
	for _, kv := range kvs {
		key := kv[:strings.Index(kv, "=")+1]
		out = slices.DeleteFunc(out, func(e string) bool { return strings.HasPrefix(e, key) })
		out = append(out, kv)
	}
	return out
}
