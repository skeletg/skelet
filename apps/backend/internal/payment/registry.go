package payment

import (
	"fmt"
	"sync"
)

// The in-memory registry of payment providers, populated at boot from two sources - offline.go's init() for the engine's own offline methods, and plugin init() calls to sdk.RegisterPaymentProvider that the host adapter translates into RegisterProvider - read-only once boot completes
var (
	regMu     sync.RWMutex
	providers = make(map[string]Registration)
	regOrder  []string // registration order; preserved for stable iteration
)

// Records a provider - panics on a duplicate slug, since two plugins claiming one is a bug the server should refuse to start with
func RegisterProvider(reg Registration) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, exists := providers[reg.Slug]; exists {
		panic(fmt.Sprintf("payment: provider %q already registered", reg.Slug))
	}
	if reg.Provider == nil {
		panic(fmt.Sprintf("payment: provider %q has nil implementation", reg.Slug))
	}
	if reg.DisplayName == "" {
		reg.DisplayName = reg.Slug
	}
	providers[reg.Slug] = reg
	regOrder = append(regOrder, reg.Slug)
}

func GetProvider(slug string) (Registration, error) {
	regMu.RLock()
	defer regMu.RUnlock()
	reg, ok := providers[slug]
	if !ok {
		return Registration{}, ErrProviderNotFound
	}
	return reg, nil
}

// Every registered provider in registration order - Sync() uses it to upsert payment_methods rows
func AllProviders() []Registration {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Registration, 0, len(regOrder))
	for _, slug := range regOrder {
		out = append(out, providers[slug])
	}
	return out
}

// Just the slug list - cheaper than AllProviders when the caller only needs identifiers, e.g. building a GraphQL enum
func RegisteredSlugs() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, len(regOrder))
	copy(out, regOrder)
	return out
}

// Drops PLUGIN registrations and restores the engine's own offline methods, which offline.go's
// init() put there and which no test means to remove - emptying outright left the registry without
// them for good, so a second run in the same binary (go test -count=2) saw a store with no way to
// pay at all
func ResetForTest() {
	regMu.Lock()
	providers = make(map[string]Registration)
	regOrder = nil
	regMu.Unlock()
	registerOfflineMethods()
}
