package delivery

import (
	"fmt"
	"slices"
	"sort"
	"sync"
)

// The in-memory registries - core fills them from init() and wiring at boot. Read-only once boot completes, like the carrier registry
var (
	regMu        sync.RWMutex
	kinds        = map[string]Kind{}
	kindOrder    []string
	pointSources = map[string]PointSourceRegistration{}
)

// RegisterKind adds a delivery kind - a duplicate slug panics, since two owners of one kind is a boot-time bug
func RegisterKind(k Kind) {
	regMu.Lock()
	defer regMu.Unlock()
	if k.Slug == "" {
		panic("delivery: kind slug is empty")
	}
	if _, dup := kinds[k.Slug]; dup {
		panic(fmt.Sprintf("delivery: kind %q already registered", k.Slug))
	}
	if k.Owner == "" {
		k.Owner = "core"
	}
	// Every kind is paid for and owes an email and a billing address - declared here so a kind can't forget them. A kind that declares billing itself keeps its floor, so store pickup can leave it to the admin
	if _, ok := k.Block(BlockEmail); !ok {
		k.Blocks = append([]BlockSpec{{Type: BlockEmail, Label: "Email", Floor: FloorLocked, Reason: "Receipt and order updates"}}, k.Blocks...)
	}
	if _, ok := k.Block(BlockPayment); !ok {
		k.Blocks = append(k.Blocks, BlockSpec{Type: BlockPayment, Label: "Payment", Floor: FloorLocked, Reason: "Every order is paid for"})
	}
	if _, ok := k.Block(BlockBillingAddress); !ok {
		k.Blocks = append(k.Blocks, BlockSpec{Type: BlockBillingAddress, Label: "Billing address", Floor: FloorLocked, Reason: "Every order requires a billing address"})
	}
	kinds[k.Slug] = k
	kindOrder = append(kindOrder, k.Slug)
}

func KindBySlug(slug string) (Kind, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	k, ok := kinds[slug]
	return k, ok
}

// Kinds in registration order
func Kinds() []Kind {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Kind, 0, len(kindOrder))
	for _, s := range kindOrder {
		out = append(out, kinds[s])
	}
	return out
}

func RegisterPointSource(r PointSourceRegistration) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := pointSources[r.Slug]; dup {
		panic(fmt.Sprintf("delivery: point source %q already registered", r.Slug))
	}
	if r.Owner == "" {
		r.Owner = "core"
	}
	pointSources[r.Slug] = r
}

func pointSourceBySlug(slug string) (PointSourceRegistration, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	r, ok := pointSources[slug]
	return r, ok
}

// PointSources registered in code, sorted by slug - remote ones are per channel and listed by the service
func PointSources() []PointSourceRegistration {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]PointSourceRegistration, 0, len(pointSources))
	for _, r := range pointSources {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// serves reports whether a registration listing kinds covers this one - an empty list serves any
func serves(regKinds []string, kind string) bool {
	return len(regKinds) == 0 || slices.Contains(regKinds, kind)
}

// resetForTest empties every registry and re-registers core's kinds - tests that register their own call it in a cleanup
func resetForTest() {
	regMu.Lock()
	kinds, kindOrder = map[string]Kind{}, nil
	pointSources = map[string]PointSourceRegistration{}
	regMu.Unlock()
	registerCoreKinds()
	registerManualPointSource()
}
