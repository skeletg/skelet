package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/node"
)

type servedShop struct{ name, dir, port string }

func (e *editor) served(a answers, c catalog, files []string) []servedShop {
	primary := primaryStorefront(a, c)
	var out []servedShop
	for _, s := range c.storefronts {
		f := s.Dir + "/.env.production"
		if !slices.Contains(files, f) {
			continue
		}
		port := e.get(f, "PORT")
		if port == "" {
			raw, _ := os.ReadFile(filepath.Join(e.root, s.Dir, "package.json"))
			port = node.DevPort(raw, "3000")
			e.set(f, "PORT", port)
		}
		shop := servedShop{s.Name, s.Dir, port}
		if s.Name == primary {
			out = append([]servedShop{shop}, out...)
		} else {
			out = append(out, shop)
		}
	}
	return out
}

const sitesDir = "deploy/caddy/sites"

var notVarChar = regexp.MustCompile(`[^A-Z0-9]+`)

func domainVar(name string) string {
	return notVarChar.ReplaceAllString(strings.ToUpper(name), "_") + "_DOMAIN"
}

func (e *editor) site(s servedShop) {
	rel := sitesDir + "/" + s.name + ".caddy"
	domain := domainVar(s.name)
	body := fmt.Sprintf("# %s at the edge, written by skelet setup - its host is %s in env/prod-caddy.env\n{$%s:%s.localhost} {\n\timport storefront %s%s\n}\n",
		s.name, domain, domain, s.name, storefrontOnHost, s.port)
	if e.create(rel, body, 0o644) {
		fmt.Fprintf(e.out, "      %s - set %s in env/prod-caddy.env\n", rel, domain)
	}
}

func (e *editor) create(rel, body string, mode fs.FileMode) bool {
	if e.err != nil {
		return false
	}
	path := filepath.Join(e.root, filepath.FromSlash(rel))
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(e.out, "      %s (exists, keeping yours)\n", rel)
		return false
	} else if !errors.Is(err, fs.ErrNotExist) {
		e.err = err
		return false
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.err = err
		return false
	}
	e.err = os.WriteFile(path, []byte(body), mode)
	return e.err == nil
}
