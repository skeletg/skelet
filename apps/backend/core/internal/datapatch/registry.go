package datapatch

import (
	"fmt"
	"sync"
)

var (
	mu         sync.RWMutex
	registered []Patch
)

// Register adds a patch to the global registry - Patches self-register from init() (core packages) or via the plugin
// SDK - Panics on a duplicate ID so collisions surface at boot, not at apply time
func Register(p Patch) {
	mu.Lock()
	defer mu.Unlock()
	if p.ID == "" {
		panic("datapatch: patch with empty ID")
	}
	if p.Apply == nil {
		panic("datapatch: patch " + p.ID + " has nil Apply")
	}
	for _, existing := range registered {
		if existing.ID == p.ID {
			panic(fmt.Sprintf("datapatch: duplicate patch ID %q", p.ID))
		}
	}
	if p.Namespace == "" {
		p.Namespace = "core"
	}
	registered = append(registered, p)
}

func Registered() []Patch {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Patch, len(registered))
	copy(out, registered)
	return out
}

// resetForTests clears the registry (test-only)
func resetForTests() {
	mu.Lock()
	defer mu.Unlock()
	registered = nil
}
