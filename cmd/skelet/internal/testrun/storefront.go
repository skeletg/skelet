package testrun

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/node"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const StorefrontAbout = `Browser tests for a storefront: skelet test storefront [name] [playwright args] - the name is optional
when only one storefront has them. Start it first with skelet dev. Playwright runs in Microsoft's image on
the stack's network, so the host needs no browser: the page comes from your dev server through
host.docker.internal, the admin API from the app container. The image tag comes from the storefront's
@playwright/test pin, since the runner and its browsers refuse to talk across versions. Writes real
customers, addresses and questions to the dev database - each run signs up throwaway accounts and makes one
throwaway superuser for the specs that set the store up.`

var playwrightPin = regexp.MustCompile(`"@playwright/test":\s*"([0-9]+(\.[0-9]+)+)"`)

func Storefront(root string, args []string) int {
	var withTests []storefronts.Storefront
	for _, sf := range storefronts.List(root) {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(sf.Dir), "playwright.config.ts")); err == nil {
			withTests = append(withTests, sf)
		}
	}
	var sf storefronts.Storefront
	// A name that's there at all is taken as the storefront, so one without browser tests is refused rather than handed
	// to Playwright as a spec filter
	if len(args) > 0 && args[0] != "" && storefronts.Exists(root, args[0]) {
		sf, _ = storefronts.Find(root, args[0])
		sf.Name, args = args[0], args[1:]
	}
	if sf.Name == "" {
		if len(withTests) > 1 {
			fmt.Fprintln(os.Stderr, "Multiple storefronts have browser tests - name one: skelet test storefront <name>")
			return 2
		}
		if len(withTests) == 1 {
			sf = withTests[0]
		}
	}
	name := sf.Name
	if !slices.Contains(withTests, sf) || name == "" {
		label := ""
		if name != "" {
			label = fmt.Sprintf("%q ", name)
		}
		fmt.Fprintf(os.Stderr, "No storefront %swith a playwright.config.ts.\n", label)
		return 1
	}
	manifest, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(sf.Dir), "package.json"))
	pin := playwrightPin.FindSubmatch(manifest)
	if pin == nil {
		fmt.Fprintf(os.Stderr, "%s/package.json has no exact @playwright/test pin\n", sf.Dir)
		return 1
	}
	lib := compose.Load(root)
	id, _ := compose.Command(root, lib.Env(), "ps", "-q", "app").Output()
	app := strings.TrimSpace(string(id))
	if app == "" || !running(app) {
		fmt.Fprintln(os.Stderr, "The backend isn't running - start the stack with skelet up")
		return 1
	}
	network, _ := exec.Command("docker", "inspect", "-f", "{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}", app).Output()
	port := node.DevPort(manifest, "3000")
	conn, err := net.Dial("tcp", "localhost:"+port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s isn't answering on :%s - start it with skelet dev %s\n", name, port, name)
		return 1
	}
	_ = conn.Close()

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
	channel, _, _ := envfile.Get(filepath.Join(root, ".env"), "STOREFRONT_CHANNEL_ID")
	if channel == "" {
		channel = defaultChannel
	}
	uid, gid, ok := compose.IDs()
	if !ok {
		uid, gid = os.Getenv("SKELET_UID"), os.Getenv("SKELET_GID")
	}
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		base = "http://host.docker.internal:" + port
	}

	argv := []string{"docker", "run", "--rm"}
	if sys.IsTerminal(os.Stdin.Fd()) {
		argv = append(argv, "-it")
	}
	argv = append(append(argv, compose.UserFlags()...),
		"-u", uid+":"+gid, "-e", "HOME=/tmp", "-e", "CI="+os.Getenv("CI"),
		"-e", "E2E_BASE_URL="+base, "-e", "E2E_ADMIN_GRAPHQL="+admin,
		"-e", "E2E_ADMIN_EMAIL="+email, "-e", "E2E_ADMIN_PASSWORD="+password, "-e", "E2E_CHANNEL_ID="+channel,
		"--network", strings.Fields(string(network) + " bridge")[0], "--add-host", "host.docker.internal:host-gateway",
		"--mount", "type=bind,src="+root+",dst="+storefronts.Mount+",readonly",
		"--mount", "type=tmpfs,dst="+storefronts.Mount+"/env",
		"--mount", "type=bind,src=/dev/null,dst="+storefronts.Mount+"/.env,readonly",
		"--mount", "type=tmpfs,dst="+storefronts.Mount+"/apps/backend",
		"--mount", "type=bind,src="+filepath.Join(root, filepath.FromSlash(sf.Dir))+",dst="+sf.ContainerDir(),
		"--ipc=host", "-w", sf.ContainerDir(), "mcr.microsoft.com/playwright:v"+string(pin[1])+"-noble",
		"node", "node_modules/@playwright/test/cli.js", "test")
	return sys.Call{Path: sys.Lookup("docker"), Argv: append(argv, args...), Env: sys.Env()}.Replace()
}

const defaultChannel = "56d051bc-0131-4476-bc0b-121d80290684"

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
