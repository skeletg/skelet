package setup

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/secret"
)

// What an app holds the same value of as core, wherever each one runs
var (
	signalsShares  = []string{"CUSTOMER_JWT_PUBLIC_KEY", "EMAIL_LINK_SECRET", "SIGNALS_SERVICE_SECRET", "TRUSTED_PROXY_KEY"}
	frontendShares = []string{"REVIEW_VOTER_TOKEN_SECRET", "STOREFRONT_REVALIDATE_SECRET", "TRUSTED_PROXY_KEY"}
	optionalShares = []string{"TRUSTED_PROXY_KEY", "ADMIN_API_SLUG"}
)

func fromCore(root string, a answers) []string {
	var keys []string
	add := func(k string) {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	if a.runs(appSignals) {
		for _, k := range signalsShares {
			add(k)
		}
	}
	for _, app := range frontends(a.apps) {
		file := appDir(root, app) + "/.env.production"
		for _, k := range frontendShares {
			if listed(root, file, k) {
				add(k)
			}
		}
		if app == "dashboard" {
			add("ADMIN_API_SLUG")
		}
	}
	return keys
}

func listed(root, file, key string) bool {
	for _, f := range []string{file, file + ".example"} {
		if _, ok, err := envfile.Get(filepath.Join(root, f), key); err == nil && ok {
			return true
		}
	}
	return false
}

func held(root string, a answers, key string) string {
	files := []string{}
	if a.runs(appSignals) {
		files = append(files, SignalsEnv)
	}
	for _, app := range frontends(a.apps) {
		files = append(files, appDir(root, app)+"/.env.production")
	}
	for _, f := range files {
		if v, _, _ := envfile.Get(filepath.Join(root, f), key); !placeholder(v) {
			return v
		}
	}
	return ""
}

func shareProblem(key, v string) string {
	switch {
	case v == "" && slices.Contains(optionalShares, key):
		return ""
	case v == "":
		return "core's value is needed - its .env has it"
	case strings.ContainsAny(v, " \t"):
		return "a value has no spaces"
	case key == "CUSTOMER_JWT_PUBLIC_KEY":
		if b, err := base64.StdEncoding.DecodeString(v); err != nil || len(b) != 32 {
			return "base64 of 32 bytes, as core's .env holds it"
		}
	}
	return ""
}

// takeShared gathers what the apps here share with core, which runs on another machine: from --shared, from what
// they already hold, or asked for - never made up, since a value core doesn't have breaks every call between them
func (a *answers) takeShared(root string, f flags, ask prompter, out io.Writer) error {
	keys := fromCore(root, *a)
	if len(keys) == 0 {
		return nil
	}
	a.shared = map[string]string{}
	given := map[string]string{}
	if f.shared != "" {
		path := f.shared
		for _, k := range append(keys, "CUSTOMER_JWT_PRIVATE_KEY") {
			if v, ok, err := envfile.Get(path, k); err != nil {
				return err
			} else if ok {
				given[k] = v
			}
		}
		if _, ok := given["CUSTOMER_JWT_PUBLIC_KEY"]; !ok && given["CUSTOMER_JWT_PRIVATE_KEY"] != "" {
			pub, err := secret.CustomerPublic(given["CUSTOMER_JWT_PRIVATE_KEY"])
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			given["CUSTOMER_JWT_PUBLIC_KEY"] = pub
		}
	}
	asked, missing := false, []string{}
	for _, k := range keys {
		if v, ok := given[k]; ok {
			if why := shareProblem(k, v); why != "" {
				fmt.Fprintf(os.Stderr, "==> %s in %s won't do - %s\n", k, f.shared, why)
				return errRefused
			}
			a.shared[k] = v
			continue
		}
		if v := held(root, *a, k); v != "" {
			a.shared[k] = v
			continue
		}
		if ask.on {
			if !asked {
				ask.section(out, "Core runs on another machine", "    These have to be core's own values - they're in its .env")
				asked = true
			}
			v := ask.text("    "+k, "", func(v string) string { return shareProblem(k, v) })
			if shareProblem(k, v) == "" {
				a.shared[k] = v
				continue
			}
		}
		if !slices.Contains(optionalShares, k) {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "==> Core runs on another machine, so these have to be its values: %s\n", strings.Join(missing, ", "))
		fmt.Fprintln(os.Stderr, "    Pass a copy of core's .env with --shared FILE, or run setup at a terminal to paste them")
		return errRefused
	}
	return nil
}
