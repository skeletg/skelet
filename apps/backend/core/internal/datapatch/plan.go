package datapatch

import (
	"fmt"
	"sort"
	"strings"
)

// plan returns the not-yet-applied patches in execution order: a patch's DependsOn edges run before it; independent patches run in stable ID order - Pure (no DB) so the ordering logic is unit-tested directly - Errors on dependency cycles and on a dependency that is neither a known patch nor an already-applied marker
func plan(patches []Patch, applied map[string]struct{}) ([]Patch, error) {
	byID := make(map[string]Patch, len(patches))
	ids := make([]string, 0, len(patches))
	for _, p := range patches {
		byID[p.ID] = p
		ids = append(ids, p.ID)
	}
	sort.Strings(ids)

	const (
		white = 0
		gray  = 1
		black = 2
	)
	state := make(map[string]int, len(patches))
	var order []Patch

	var visit func(id string, stack []string) error
	visit = func(id string, stack []string) error {
		switch state[id] {
		case black:
			return nil
		case gray:
			return fmt.Errorf("datapatch: dependency cycle: %s", strings.Join(append(stack, id), " -> "))
		}
		state[id] = gray
		p := byID[id]
		deps := append([]string(nil), p.DependsOn...)
		sort.Strings(deps)
		for _, dep := range deps {
			if _, known := byID[dep]; !known {
				if _, isApplied := applied[dep]; isApplied {
					continue // satisfied by an applied patch no longer in the registry
				}
				return fmt.Errorf("datapatch: patch %q depends on unknown patch %q", id, dep)
			}
			if err := visit(dep, append(stack, id)); err != nil {
				return err
			}
		}
		state[id] = black
		order = append(order, p)
		return nil
	}

	for _, id := range ids {
		if err := visit(id, nil); err != nil {
			return nil, err
		}
	}

	var pending []Patch
	for _, p := range order {
		if _, done := applied[p.ID]; !done {
			pending = append(pending, p)
		}
	}
	return pending, nil
}
