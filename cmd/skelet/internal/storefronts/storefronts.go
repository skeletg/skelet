// Package storefronts says which storefronts a checkout has: the project's own under apps/channels/, and the templates
// they're made from under templates/ - each a directory holding a package.json, and each a compose service of its name
package storefronts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/gitx"
)

// The two homes, in the order they're listed - the project's own first
var homes = []string{"apps/channels", "templates"}

// Storefront is one of them. Dir is relative to the checkout root, slash-separated
type Storefront struct {
	Name, Dir string
}

// Mount is where a storefront's dev container sees the checkout - the whole workspace, so the packages resolve
const Mount = "/var/www/skelet"

// ContainerDir is the storefront's directory inside its dev container
func (s Storefront) ContainerDir() string { return Mount + "/" + s.Dir }

// Template says the storefront is one of upstream's templates rather than the project's own
func (s Storefront) Template() bool { return strings.HasPrefix(s.Dir, "templates/") }

// List is every storefront, the project's own before the templates, each sorted by name
func List(root string) []Storefront {
	var out []Storefront
	for _, home := range homes {
		manifests, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(home), "*", "package.json"))
		slices.Sort(manifests)
		for _, m := range manifests {
			name := filepath.Base(filepath.Dir(m))
			out = append(out, Storefront{Name: name, Dir: home + "/" + name})
		}
	}
	return out
}

// Find is the storefront called name
func Find(root, name string) (Storefront, bool) {
	i := slices.IndexFunc(List(root), func(s Storefront) bool { return s.Name == name })
	if i < 0 {
		return Storefront{}, false
	}
	return List(root)[i], true
}

// InHead is the storefronts git's HEAD has, checked out or not - one setup left out of the working tree is still there
// to pick, and still there to fork
func InHead(root string) []Storefront {
	var out []Storefront
	for _, home := range homes {
		for _, n := range gitx.TreeDirs(root, home) {
			if _, err := gitx.Show(root, home+"/"+n+"/package.json"); err == nil {
				out = append(out, Storefront{Name: n, Dir: home + "/" + n})
			}
		}
	}
	return out
}

// Known is List, then what HEAD has that the working tree doesn't
func Known(root string) []Storefront {
	out := List(root)
	for _, s := range InHead(root) {
		if !slices.ContainsFunc(out, func(o Storefront) bool { return o.Name == s.Name }) {
			out = append(out, s)
		}
	}
	return out
}

// FindKnown is the storefront called name, on disk or only in HEAD
func FindKnown(root, name string) (Storefront, bool) {
	list := Known(root)
	i := slices.IndexFunc(list, func(s Storefront) bool { return s.Name == name })
	if i < 0 {
		return Storefront{}, false
	}
	return list[i], true
}

// Names is List's names, in its order
func Names(root string) []string {
	var out []string
	for _, s := range List(root) {
		out = append(out, s.Name)
	}
	return out
}

// Packages is every package the storefronts share - packages/<name> with a package.json - as slash-separated dirs
func Packages(root string) []string {
	manifests, _ := filepath.Glob(filepath.Join(root, "packages", "*", "package.json"))
	slices.Sort(manifests)
	var out []string
	for _, m := range manifests {
		out = append(out, "packages/"+filepath.Base(filepath.Dir(m)))
	}
	return out
}

// HasScript says the project at dir has a package.json script by that name
func HasScript(root, dir, script string) bool {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), "package.json"))
	if err != nil {
		return false
	}
	var m struct{ Scripts map[string]string }
	return json.Unmarshal(raw, &m) == nil && m.Scripts[script] != ""
}

// Homes are the directories storefronts live in, for a caller listing what git has but the working tree doesn't
func Homes() []string { return slices.Clone(homes) }

// Exists says a directory under one of the homes is there at all, package.json or not - for a name the scaffold must
// refuse
func Exists(root, name string) bool {
	for _, home := range homes {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(home), name)); err == nil {
			return true
		}
	}
	return false
}
