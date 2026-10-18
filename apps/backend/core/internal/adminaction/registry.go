package adminaction

import (
	"fmt"
	"strings"
	"sync"
)

var (
	mu       sync.RWMutex
	registry []Definition
)

// Not safe from init() - handlers close over services that don't exist yet,
// so domain packages call this from RegisterActions(svc) once main.go is done
func Register(d Definition) {
	if err := validateDefinition(d); err != nil {
		panic(fmt.Sprintf("action: register %q: %v", d.Slug, err))
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range registry {
		if existing.Slug == d.Slug {
			panic(fmt.Sprintf("action: slug %q already registered", d.Slug))
		}
	}
	registry = append(registry, d)
}

func AllDefinitions() []Definition {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Definition, len(registry))
	copy(out, registry)
	return out
}

func Find(slug string) (Definition, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, d := range registry {
		if d.Slug == slug {
			return d, true
		}
	}
	return Definition{}, false
}

func Validate() error {
	return nil
}

func validateDefinition(d Definition) error {
	if d.Slug == "" {
		return fmt.Errorf("slug is required")
	}
	if d.Slug != strings.ToLower(d.Slug) {
		return fmt.Errorf("slug %q must be lowercase", d.Slug)
	}
	if strings.ContainsAny(d.Slug, " \t\n") {
		return fmt.Errorf("slug %q contains whitespace", d.Slug)
	}
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(d.Permission) == "" {
		return fmt.Errorf("permission is required")
	}
	if d.Handler == nil {
		return fmt.Errorf("handler is required")
	}
	seen := make(map[string]bool, len(d.Arguments))
	for _, a := range d.Arguments {
		if a.Name == "" {
			return fmt.Errorf("argument name is required")
		}
		if seen[a.Name] {
			return fmt.Errorf("duplicate argument %q", a.Name)
		}
		seen[a.Name] = true
		if a.Type == ArgSelect && len(a.Options) == 0 {
			return fmt.Errorf("argument %q: select type requires at least one option", a.Name)
		}
	}
	return nil
}
