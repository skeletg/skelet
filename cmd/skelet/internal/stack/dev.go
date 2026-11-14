package stack

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/node"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const DevAbout = `The frontends' dev servers on the host: skelet dev [app...]. With no names it runs the apps this
clone was set up with (skelet setup --apps), else the dashboard and every storefront. One app becomes its dev
server outright; several run side by side with their output prefixed, and Ctrl-C - or any one of them exiting -
stops them all. Each listens on the port its package.json dev script names, so two storefronts never collide.

The root .env is laid over your environment first, so the channel id and the secrets core shares with the
storefront live in one place. Missing dependencies are installed first.`

// Dev defaults, used only when the root .env is silent
var devDefaults = []string{"NEXT_PUBLIC_BACKEND_URL=http://localhost:8080", "STOREFRONT_REVALIDATE_SECRET=dev-revalidate-secret-change-me",
	"STOREFRONT_CHANNEL_ID=56d051bc-0131-4476-bc0b-121d80290684"}

type devApp struct {
	name, dir, port string
	manifest        []byte
}

func Dev(root string, args []string) int {
	names := args
	if len(names) == 0 {
		names = defaultApps(root)
	}
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "No frontends in this clone - skelet setup --apps ... picks them")
		return 1
	}
	var apps []devApp
	for _, name := range names {
		app, ok := findApp(root, name)
		if !ok {
			return devUsage(root)
		}
		apps = append(apps, app)
	}
	if code := checkPorts(apps); code != 0 {
		return code
	}
	env, err := devEnv(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet dev:", err)
		return 1
	}
	for _, app := range apps {
		if code := prepare(root, app, env); code != 0 {
			return code
		}
	}
	if len(apps) == 1 {
		fmt.Printf("==> %s on the host (http://localhost:%s)\n", apps[0].name, apps[0].port)
		return sys.Call{Dir: filepath.Join(root, apps[0].dir), Path: sys.Lookup("pnpm"), Argv: []string{"pnpm", "run", "dev"}, Env: env}.Replace()
	}
	return together(root, apps, env)
}

func defaultApps(root string) []string {
	present := append([]string{}, storefronts.Names(root)...)
	if _, err := os.Stat(filepath.Join(root, "apps", "dashboard", "package.json")); err == nil {
		present = append([]string{"dashboard"}, present...)
	}
	if picked, chosen := profile.Current(root).Apps(); chosen {
		return slices.DeleteFunc(slices.Clone(picked), func(app string) bool { return !slices.Contains(present, app) })
	}
	return present
}

func findApp(root, name string) (devApp, bool) {
	dir, fallback := "", "3000"
	if name == "dashboard" {
		dir, fallback = filepath.Join("apps", "dashboard"), "5173"
	} else if s, ok := storefronts.Find(root, name); ok {
		dir = filepath.FromSlash(s.Dir)
	} else {
		fmt.Fprintf(os.Stderr, "No such app: %s\n", name)
		return devApp{}, false
	}
	manifest, err := os.ReadFile(filepath.Join(root, dir, "package.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s has no package.json - see skelet setup\n", dir)
		return devApp{}, false
	}
	return devApp{name: name, dir: dir, port: node.DevPort(manifest, fallback), manifest: manifest}, true
}

func checkPorts(apps []devApp) int {
	taken := map[string]string{}
	for _, app := range apps {
		if other, clash := taken[app.port]; clash {
			fmt.Fprintf(os.Stderr, "%s and %s both listen on %s - give one another port in its package.json dev script\n", other, app.name, app.port)
			return 1
		}
		taken[app.port] = app.name
		l, err := net.Listen("tcp", ":"+app.port)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Port %s, %s's, is already taken - a dev server still running, or a container from before the frontends left Docker (skelet up removes those)\n", app.port, app.name)
			return 1
		}
		_ = l.Close()
	}
	return 0
}

// devEnv is the caller's environment with the root .env laid over it, as compose reads that file - parsed, not
// sourced - and the dev defaults filling whatever is still empty
func devEnv(root string) ([]string, error) {
	pairs, err := envfile.Pairs(filepath.Join(root, ".env"))
	if err != nil {
		return nil, err
	}
	env := append(sys.Env(), "LC_ALL=C.UTF-8", "LANG=C.UTF-8")
	for _, kv := range pairs {
		env = setEnv(env, kv, true)
	}
	for _, kv := range devDefaults {
		env = setEnv(env, kv, false)
	}
	return env, nil
}

// setEnv puts KEY=value in env - over what's there, or only when that is missing or empty
func setEnv(env []string, kv string, override bool) []string {
	key := kv[:strings.Index(kv, "=")+1]
	i := slices.IndexFunc(env, func(e string) bool { return strings.HasPrefix(e, key) })
	switch {
	case i < 0:
		return append(env, kv)
	case override || env[i] == key:
		env[i] = kv
	}
	return env
}

func prepare(root string, app devApp, env []string) int {
	switch have, problem := node.Pnpm(root, app.dir, node.Pin(app.manifest)); {
	case problem != "" && have == "":
		fmt.Fprintf(os.Stderr, "Can't run %s: %s\n", app.name, problem)
		return 1
	case problem != "":
		fmt.Fprintf(os.Stderr, "==> %s: %s\n", app.name, problem)
	}
	abs := filepath.Join(root, app.dir)
	if node.Unwritable(abs) {
		fmt.Fprintf(os.Stderr, "%s/node_modules isn't yours to write - a frontend container left it behind. skelet fixperms gives it back\n", app.dir)
		return 1
	}
	switch node.Tree(abs) {
	case "none":
		fmt.Printf("==> installing %s deps\n", app.name)
		install := exec.Command("pnpm", "install", "--frozen-lockfile")
		install.Dir, install.Env = abs, env
		install.Stdin, install.Stdout, install.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := sys.Run(install); err != nil {
			return sys.ExitCode(err)
		}
	case "npm":
		fmt.Fprintf(os.Stderr, "%s/node_modules is an npm install from before the switch to pnpm - replace it:\n", app.dir)
		fmt.Fprintf(os.Stderr, "    rm -rf %s/node_modules && (cd %s && pnpm install)\n", app.dir, app.dir)
		return 1
	}
	return 0
}

// together runs every app in a process group of its own, so a stop reaches each one's whole tree and not this process
func together(root string, apps []devApp, env []string) int {
	width := 0
	for _, app := range apps {
		width = max(width, len(app.name))
	}
	color := sys.IsTerminal(os.Stdout.Fd())
	var out sync.Mutex
	cmds := make([]*exec.Cmd, len(apps))
	done := make(chan int, len(apps))
	for i, app := range apps {
		cmd := exec.Command(sys.Lookup("pnpm"), "run", "dev")
		cmd.Dir, cmd.Env = filepath.Join(root, app.dir), append(slices.Clone(env), "FORCE_COLOR=1")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		label := fmt.Sprintf("%-*s |", width, app.name)
		if color {
			label = fmt.Sprintf("\x1b[%dm%s\x1b[0m", 31+i%6, label)
		}
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "skelet dev: %s: %v\n", app.name, err)
			stop(cmds, syscall.SIGTERM)
			return 1
		}
		cmds[i] = cmd
		fmt.Printf("==> %s on http://localhost:%s\n", app.name, app.port)
		var copied sync.WaitGroup
		for _, r := range []io.Reader{stdout, stderr} {
			copied.Go(func() { prefix(r, label, &out) })
		}
		go func() {
			copied.Wait()
			done <- sys.ExitCode(cmd.Wait())
		}()
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	code, left := 0, len(apps)
	select {
	case s := <-signals:
		code = 128 + int(s.(syscall.Signal))
		stop(cmds, s.(syscall.Signal))
	case code = <-done:
		left--
		stop(cmds, syscall.SIGINT)
	}
	deadline := time.After(10 * time.Second)
	for left > 0 {
		select {
		case <-done:
			left--
		case <-signals:
			stop(cmds, syscall.SIGTERM)
		case <-deadline:
			stop(cmds, syscall.SIGKILL)
			deadline = nil
		}
	}
	return code
}

func stop(cmds []*exec.Cmd, sig syscall.Signal) {
	for _, cmd := range cmds {
		if cmd != nil && cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, sig)
		}
	}
}

func prefix(r io.Reader, label string, out *sync.Mutex) {
	lines := bufio.NewScanner(r)
	lines.Buffer(make([]byte, 64*1024), 1024*1024)
	for lines.Scan() {
		out.Lock()
		fmt.Println(label, lines.Text())
		out.Unlock()
	}
	if err := lines.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
		out.Lock()
		fmt.Println(label, "(output stopped:", err.Error()+")")
		out.Unlock()
	}
}

func devUsage(root string) int {
	apps := append([]string{"dashboard"}, storefronts.Names(root)...)
	fmt.Fprintln(os.Stderr, "usage: skelet dev [app...]")
	fmt.Fprintln(os.Stderr, "  "+strings.Join(apps, ", "))
	return 2
}
