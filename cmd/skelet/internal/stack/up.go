package stack

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
	"github.com/skeletg/skelet/cmd/skelet/internal/secret"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const UpAbout = `Starts the dev stack, and is safe to re-run: compose only touches what changed. --lean leaves out
the sidecars (OpenSearch, the Signals process and its ClickHouse) and the dev tools .skelet-tools picks;
your apps still start. Any other argument goes to docker compose up - skelet up mailpit starts one tool.

Before starting it syncs env/signals.env from env/go.env and points the env files at the dev tools you
picked, then checks the ports the stack publishes: one something else holds is listed with whoever holds
it, and on a terminal you can free it or move the service through .env. Frontends that run on the host get
the commands that start them, and a container left holding one of their ports is stopped.`

func Up(root string, args []string) int {
	lean := len(args) > 0 && args[0] == "--lean"
	if lean {
		args = args[1:]
	}
	if code := needsDevEnv(root); code != 0 {
		return code
	}
	lib := compose.Load(root)
	if lean {
		lib = compose.LoadLean(root)
	}
	if err := syncSignalsEnv(root); err != nil {
		fmt.Fprintln(os.Stderr, "skelet up:", err)
		return 1
	}
	if err := syncToolEnv(root, lib.Picked); err != nil {
		fmt.Fprintln(os.Stderr, "skelet up:", err)
		return 1
	}
	sys.HandleInterrupts()
	if !preflight(root, lib, prompter{in: os.Stdin, on: sys.IsTerminal(os.Stdin.Fd())}) {
		return 1
	}
	// --build every time - a cache hit when nothing changed, and without it an image from an older checkout (the
	// project name is pinned, so another clone's counts) runs on after a pull changed its Dockerfile or go.mod. A cache
	// hit only keeps the containers if the image ID holds too, and buildx's default provenance attestation gives every
	// build a new one, which recreated the app on every run. These images never leave the machine
	mountpoints(root, lib)
	env := lib.CreateEnv()
	if _, set := os.LookupEnv("BUILDX_NO_DEFAULT_ATTESTATIONS"); !set {
		env = append(env, "BUILDX_NO_DEFAULT_ATTESTATIONS=1")
	}
	up := compose.Call(root, env, append([]string{"up", "-d", "--build"}, args...)...)
	if err := up.Run(); err != nil {
		return sys.ExitCode(err)
	}
	if lib.Apps == "" && len(args) == 0 {
		hostFrontends(root, lib)
	}
	return 0
}

// mountpoints makes what a Docker storefront's mounts land on - a missing one Docker makes as root, or can't make at all under the read-only mount
func mountpoints(root string, lib compose.Lib) {
	list := storefronts.List(root)
	for app := range strings.SplitSeq(lib.Apps, ",") {
		s, ok := storefronts.Named(list, app)
		if !ok {
			continue
		}
		for _, dir := range []string{"node_modules", "apps/channels", s.Dir + "/.next"} {
			_ = os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755)
		}
		if f, err := os.OpenFile(filepath.Join(root, ".env"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600); err == nil {
			_ = f.Close()
		}
	}
}

// needsDevEnv refuses a clone without the env files the dev compose file names, which only setup's dev target
// writes - compose would only say an env file is missing, not that the clone is set up for another target
func needsDevEnv(root string) int {
	var missing []string
	for _, f := range []string{"env/go.env", "env/db.env"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		return 0
	}
	fmt.Fprintf(os.Stderr, "skelet up needs %s, which this clone doesn't have.\n", strings.Join(missing, " "))
	target := "dev"
	if p, err := profile.Load(filepath.Join(root, profile.File)); err == nil {
		if t, _ := p.Get("SKELET_TARGET"); t != "" {
			target = t
		}
	}
	if target != "dev" {
		fmt.Fprintf(os.Stderr, "This clone is set up for the '%s' target - skelet up runs the DEV stack.\n", target)
		fmt.Fprintf(os.Stderr, "Re-run skelet setup and pick 'dev', or deploy with docker-compose.%s.yml (docs/deploying.md).\n", target)
	} else {
		fmt.Fprintln(os.Stderr, "Run skelet setup to write the env files.")
	}
	return 1
}

// syncSignalsEnv keeps env/signals.env, the Signals process's own file, holding what it shares with core - the
// customer PUBLIC key, never the private one - plus a key of its own that core never holds. Read the way compose
// reads go.env, so the suite gets the value core runs with
func syncSignalsEnv(root string) error {
	goEnv, sig := filepath.Join(root, "env", "go.env"), filepath.Join(root, "env", "signals.env")
	if _, err := os.Stat(goEnv); err != nil {
		return nil
	}
	if _, err := os.Stat(sig); err != nil {
		raw, err := os.ReadFile(filepath.Join(root, "env", "signals.env.example"))
		if err != nil {
			return fmt.Errorf("env/signals.env needs its template: %w", err)
		}
		if err := envfile.WriteAtomic(sig, raw, 0o600); err != nil {
			return err
		}
		fmt.Println("==> wrote env/signals.env from its template")
	}
	// JWT_SECRET is retired, and the suite refuses to boot while it's set
	if err := envfile.Del(sig, "JWT_SECRET"); err != nil {
		return err
	}
	if v, _, err := envfile.Get(sig, "SIGNALS_ENCRYPTION_KEY"); err != nil {
		return err
	} else if v == "" {
		key, err := secret.Key32(rand.Reader)
		if err != nil {
			return err
		}
		if err := envfile.Set(sig, "SIGNALS_ENCRYPTION_KEY", key); err != nil {
			return err
		}
		fmt.Println("==> generated SIGNALS_ENCRYPTION_KEY in env/signals.env")
	}
	for _, key := range []string{"CUSTOMER_JWT_PUBLIC_KEY", "EMAIL_LINK_SECRET", "SIGNALS_SERVICE_SECRET"} {
		v, _, err := envfile.Get(goEnv, key)
		if err != nil {
			return err
		}
		if v == "" {
			continue
		}
		if err := envfile.Set(sig, key, v); err != nil {
			return err
		}
	}
	return nil
}

// syncToolEnv points the backend env files at the dev tools picked and away from the ones that aren't. Only values a
// tool owns are touched, so an SMTP relay or an OTLP collector of your own is left alone
func syncToolEnv(root, picked string) error {
	on := func(tool string) bool { return slices.Contains(strings.Split(picked, ","), tool) }
	for _, name := range []string{"go.env", "signals.env"} {
		f := filepath.Join(root, "env", name)
		if _, err := os.Stat(f); err != nil {
			continue
		}
		var sets [][2]string
		smtp, _, err := envfile.Get(f, "SMTP_HOST")
		if err != nil {
			return err
		}
		switch {
		case on("mailpit") && (smtp == "" || smtp == "mailpit"):
			sets = append(sets, [2]string{"SMTP_HOST", "mailpit"}, [2]string{"SMTP_PORT", "1025"})
		case !on("mailpit") && smtp == "mailpit":
			sets = append(sets, [2]string{"SMTP_HOST", ""})
		}
		if otlp, _, _ := envfile.Get(f, "OTEL_EXPORTER_OTLP_ENDPOINT"); otlp == "" || otlp == "http://grafana:4318" {
			endpoint, exporter := "", ""
			if on("grafana") {
				endpoint, exporter = "http://grafana:4318", "otlp"
			}
			sets = append(sets, [2]string{"OTEL_EXPORTER_OTLP_ENDPOINT", endpoint},
				[2]string{"OTEL_TRACES_EXPORTER", exporter}, [2]string{"OTEL_METRICS_EXPORTER", exporter})
		}
		for _, kv := range sets {
			if err := envfile.Set(f, kv[0], kv[1]); err != nil {
				return err
			}
		}
	}
	return nil
}

// hostFrontends says how to start the frontends that run on the host, and stops a container of theirs left over from
// when they ran in Docker - it holds the port, which reads as a host dev server that won't start
func hostFrontends(root string, lib compose.Lib) {
	fmt.Println()
	fmt.Println("Frontends are not in Docker (no .skelet-apps) - run them on the host:")
	if _, err := os.Stat(filepath.Join(root, "apps", "dashboard", "package.json")); err == nil {
		fmt.Println("    skelet serve dashboard        → http://localhost:5173")
	}
	fronts := []string{"dashboard"}
	for _, s := range storefronts.List(root) {
		fmt.Printf("    skelet serve %s     → http://localhost:3000\n", s.Name)
		fronts = append(fronts, s.Name)
	}
	fmt.Println("To run them in Docker instead, run skelet setup --docker-apps <names>")
	// Every frontend this checkout could run, since a shop skelet add made leaves a container holding its port too
	for _, f := range leftovers(root, lib, fronts) {
		fmt.Printf("    stopping the %s container - not listed in .skelet-apps, and it holds the port\n", f)
		_ = compose.Command(root, lib.Env(), "--profile", f, "stop", f).Run()
	}
}

// leftovers is which of fronts this project has a running container for. Asked with each one's profile, since a
// frontend that isn't in .skelet-apps is outside the profiles compose would otherwise look at
func leftovers(root string, lib compose.Lib, fronts []string) []string {
	args := []string{}
	for _, f := range fronts {
		args = append(args, "--profile", f)
	}
	out, _ := compose.Command(root, lib.Env(), append(args, "ps", "--services", "--status", "running")...).Output()
	running := strings.Fields(string(out))
	var found []string
	for _, f := range fronts {
		if slices.Contains(running, f) {
			found = append(found, f)
		}
	}
	return found
}
