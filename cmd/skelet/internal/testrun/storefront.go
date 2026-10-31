package testrun

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const StorefrontAbout = `Browser tests for a storefront: skelet test storefront [name] [playwright args] - the name is optional
when only one storefront has them. Playwright runs in Microsoft's image, inside the storefront dev
container's network namespace, so the host needs no Node and no browser and the page is served as
localhost. The image tag comes from the storefront's @playwright/test pin, since the runner and its
browsers refuse to talk across versions. Writes real customers, addresses and questions to the dev
database - each run signs up throwaway accounts and makes one throwaway superuser for the specs
that set the store up.`

var playwrightPin = regexp.MustCompile(`"@playwright/test":\s*"([0-9]+(\.[0-9]+)+)"`)

func Storefront(root string, args []string) int {
	name := ""
	if len(args) > 0 && args[0] != "" {
		if fi, err := os.Stat(filepath.Join(root, "apps", "channels", args[0])); err == nil && fi.IsDir() {
			name, args = args[0], args[1:]
		}
	}
	if name == "" {
		configs, _ := filepath.Glob(filepath.Join(root, "apps", "channels", "*", "playwright.config.ts"))
		if len(configs) > 1 {
			fmt.Fprintln(os.Stderr, "Multiple storefronts have browser tests - name one: skelet test storefront <name>")
			return 2
		}
		if len(configs) == 1 {
			name = filepath.Base(filepath.Dir(configs[0]))
		}
	}
	dir := filepath.Join(root, "apps", "channels", name)
	if _, err := os.Stat(filepath.Join(dir, "playwright.config.ts")); name == "" || err != nil {
		label := ""
		if name != "" {
			label = fmt.Sprintf("%q ", name)
		}
		fmt.Fprintf(os.Stderr, "No storefront %swith a playwright.config.ts under apps/channels/.\n", label)
		return 1
	}
	manifest, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	pin := playwrightPin.FindSubmatch(manifest)
	if pin == nil {
		fmt.Fprintf(os.Stderr, "apps/channels/%s/package.json has no exact @playwright/test pin\n", name)
		return 1
	}
	lib := compose.Load(root)
	id, _ := compose.Command(root, lib.Env(), "ps", "-q", name).Output()
	container := strings.TrimSpace(string(id))
	if container == "" || !running(container) {
		fmt.Fprintf(os.Stderr, "%s isn't running - start the stack with skelet up\n", name)
		return 1
	}

	// Specs that set the store up sign in to the admin API as a superuser made for this run - like the throwaway
	// customers, one inert e2e-storefront-* admin account per run stays behind
	email, password := throwaway()
	create := compose.Command(root, lib.Env(), append(append([]string{"exec", "-T"}, compose.UserFlags()...),
		"app", "go", "run", "./cmd/cli", "admin", "create-superuser",
		"--email", email, "--password", password, "--first-name", "E2E", "--last-name", "Storefront")...)
	create.Stderr = os.Stderr
	if err := sys.Run(create); err != nil {
		return sys.ExitCode(err)
	}
	slug, _ := compose.Command(root, lib.Env(), "exec", "-T", "app", "printenv", "ADMIN_API_SLUG").Output()
	admin := "http://app:8080/api/admin/graphql"
	if s := strings.TrimSpace(string(slug)); s != "" {
		admin = "http://app:8080/api/admin/" + s + "/graphql"
	}
	envs, _ := exec.Command("docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", container).Output()
	channel := ""
	for line := range strings.Lines(string(envs)) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "STOREFRONT_CHANNEL_ID="); ok {
			channel = v
		}
	}
	uid, gid, ok := compose.IDs()
	if !ok {
		uid, gid = os.Getenv("SKELET_UID"), os.Getenv("SKELET_GID")
	}
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		base = "http://localhost:3000"
	}

	// --volumes-from brings the source and the container's node_modules volume, so the specs resolve @playwright/test
	// from the tree the dev server uses. --network container: shares the storefront's network namespace, so the
	// browser reaches it as localhost - next dev serves its chunks only there
	argv := []string{"docker", "run", "--rm"}
	if sys.IsTerminal(os.Stdin.Fd()) {
		argv = append(argv, "-it")
	}
	argv = append(append(argv, compose.UserFlags()...),
		"-u", uid+":"+gid, "-e", "HOME=/tmp", "-e", "CI="+os.Getenv("CI"),
		"-e", "E2E_BASE_URL="+base, "-e", "E2E_ADMIN_GRAPHQL="+admin,
		"-e", "E2E_ADMIN_EMAIL="+email, "-e", "E2E_ADMIN_PASSWORD="+password, "-e", "E2E_CHANNEL_ID="+channel,
		"--network", "container:"+container, "--volumes-from", container, "--ipc=host",
		"-w", "/var/www/storefront", "mcr.microsoft.com/playwright:v"+string(pin[1])+"-noble",
		"node", "node_modules/@playwright/test/cli.js", "test")
	return sys.Call{Path: sys.Lookup("docker"), Argv: append(argv, args...), Env: sys.Env()}.Replace()
}

func running(container string) bool {
	state, _ := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", container).Output()
	return strings.TrimSpace(string(state)) == "true"
}

// throwaway is a fresh superuser's address and password for one run
func throwaway() (string, string) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("e2e-storefront-%d-%s@skelet.test", time.Now().Unix(), hex.EncodeToString(b[:3])), hex.EncodeToString(b)
}
