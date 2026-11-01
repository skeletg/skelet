package testrun

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
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
	id, _ := compose.Command(root, lib.Env(), "ps", "-q", name).Output()
	container := strings.TrimSpace(string(id))
	if container == "" || !running(container) {
		fmt.Fprintf(os.Stderr, "%s isn't running - start the stack with skelet up\n", name)
		return 1
	}
	mnt, err := mounts(container)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't read the %s container's mounts: %v\n", name, err)
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

	// The container's own mounts bring the source and its node_modules volume, so the specs resolve @playwright/test
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
		"--network", "container:"+container)
	argv = append(append(argv, mnt...), "--ipc=host",
		"-w", sf.ContainerDir(), "mcr.microsoft.com/playwright:v"+string(pin[1])+"-noble",
		"node", "node_modules/@playwright/test/cli.js", "test")
	return sys.Call{Path: sys.Lookup("docker"), Argv: append(argv, args...), Env: sys.Env()}.Replace()
}

// mounts is a container's mounts as docker run flags. --volumes-from can't take them over: it looks a tmpfs up as a
// volume and finds none, and the tmpfs over env/ is what keeps the backend's secrets from the specs, as from the server
func mounts(container string) ([]string, error) {
	raw, err := exec.Command("docker", "inspect", "-f", "{{json .Mounts}}", container).Output()
	if err != nil {
		return nil, err
	}
	var ms []mount
	if err := json.Unmarshal(raw, &ms); err != nil {
		return nil, err
	}
	return mountFlags(ms)
}

type mount struct {
	Type, Name, Source, Destination string
	RW                              bool
}

// mountFlags refuses a mount kind it doesn't know - a dropped mask would hand the specs what it hid
func mountFlags(ms []mount) ([]string, error) {
	var out []string
	for _, m := range ms {
		var spec []string
		switch m.Type {
		case "bind":
			spec = []string{"type=bind", "src=" + m.Source}
		case "volume":
			spec = []string{"type=volume", "src=" + m.Name}
		case "tmpfs":
			spec = []string{"type=tmpfs"}
		default:
			return nil, fmt.Errorf("a %s mount at %s, which the test container can't be given", m.Type, m.Destination)
		}
		spec = append(spec, "dst="+m.Destination)
		if !m.RW && m.Type != "tmpfs" {
			spec = append(spec, "readonly")
		}
		for i, f := range spec {
			if strings.ContainsAny(f, `,"`) {
				spec[i] = `"` + strings.ReplaceAll(f, `"`, `""`) + `"`
			}
		}
		out = append(out, "--mount", strings.Join(spec, ","))
	}
	return out, nil
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
