package dataio

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// The set of live resources. An instance rather than a package global so a test (or a process wiring a
// different subset) can't inherit another's entries - the composition root registers into one
type Registry struct {
	mu        sync.RWMutex
	resources []Resource
	sources   map[string]ChoiceProvider
}

func NewRegistry() *Registry { return &Registry{} }

// Panics on a bad or duplicate registration - two packages claiming "products" is a wiring bug, not a runtime condition
func (r *Registry) Register(res Resource) {
	if err := validate(res); err != nil {
		panic(fmt.Sprintf("dataio: register %q: %v", res.Slug, err))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.resources {
		if existing.Slug == res.Slug {
			panic(fmt.Sprintf("dataio: resource %q already registered", res.Slug))
		}
	}
	r.resources = append(r.resources, res)
}

// Sorted by group then label, so the admin nav and CLI output stay stable across boots regardless of registration order
func (r *Registry) List() []Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := append([]Resource(nil), r.resources...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func (r *Registry) Get(slug string) (Resource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, res := range r.resources {
		if res.Slug == slug {
			return res, nil
		}
	}
	return Resource{}, fmt.Errorf("%w %q", ErrUnknownResource, slug)
}

func (r *Registry) Slugs() []string {
	list := r.List()
	out := make([]string, len(list))
	for i, res := range list {
		out[i] = res.Slug
	}
	return out
}

func validate(res Resource) error {
	switch {
	case res.Slug == "":
		return fmt.Errorf("slug is required")
	case res.Label == "":
		return fmt.Errorf("label is required")
	case res.ViewPermission == "":
		return fmt.Errorf("a view permission is required")
	case res.NewExporter == nil && res.NewImporter == nil:
		return fmt.Errorf("neither importable nor exportable")
	case res.NewImporter != nil && res.ImportPermission == "":
		return fmt.Errorf("an importer needs an import permission")
	case len(res.Fields) == 0:
		return fmt.Errorf("at least one field is required")
	}
	seen := map[string]bool{}
	for _, f := range res.Fields {
		if f.Name == "" {
			return fmt.Errorf("a field is missing its name")
		}
		if f.Name != strings.ToLower(f.Name) || strings.ContainsAny(f.Name, " \t") {
			return fmt.Errorf("field %q must be lowercase and space-free", f.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("duplicate field %q", f.Name)
		}
		seen[f.Name] = true
	}
	for _, f := range res.Fields {
		for _, a := range f.Aliases {
			if seen[a] || a != strings.ToLower(a) {
				return fmt.Errorf("field %q alias %q must be lowercase and unique", f.Name, a)
			}
			seen[a] = true
		}
	}
	// An importer with no identifier has nothing to upsert on, so every row would create a duplicate
	if res.NewImporter != nil && len(res.identifiers()) == 0 {
		return fmt.Errorf("an importer needs at least one Identifier field")
	}
	optSeen := map[string]bool{}
	for _, o := range res.Options {
		if o.Name == "" {
			return fmt.Errorf("an option is missing its name")
		}
		if optSeen[o.Name] {
			return fmt.Errorf("duplicate option %q", o.Name)
		}
		optSeen[o.Name] = true
		// A select with nothing to select from renders as an empty dropdown nobody can satisfy
		if o.Type == OptSelect && len(o.Choices) == 0 && o.Source == "" {
			return fmt.Errorf("option %q is a select with neither choices nor a source", o.Name)
		}
	}
	return nil
}

// Adds a named choice provider - call once the services it closes over exist, same phase as Register
func (r *Registry) RegisterSource(name string, p ChoiceProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sources == nil {
		r.sources = map[string]ChoiceProvider{}
	}
	if _, dup := r.sources[name]; dup {
		panic(fmt.Sprintf("dataio: choice source %q already registered", name))
	}
	r.sources[name] = p
}

// Resolves an option's choices, preferring its Source over any static Choices
func (r *Registry) Choices(ctx context.Context, o OptionDef) ([]Choice, error) {
	if o.Source == "" {
		return o.Choices, nil
	}
	r.mu.RLock()
	p, ok := r.sources[o.Source]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown choice source %q on option %q", o.Source, o.Name)
	}
	return p(ctx)
}

// Every Source named by a registered option must exist. Called at wiring time AFTER the providers register, so a
// typo fails the boot - config's equivalent validates too early and degrades into a silently empty dropdown
func (r *Registry) ValidateSources() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, res := range r.resources {
		for _, o := range res.Options {
			if o.Source == "" {
				continue
			}
			if _, ok := r.sources[o.Source]; !ok {
				return fmt.Errorf("resource %q option %q names unregistered choice source %q", res.Slug, o.Name, o.Source)
			}
		}
	}
	return nil
}
