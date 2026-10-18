package shipping

import (
	"errors"
	"fmt"
	"sync"
)

// The in-memory carrier registry - the built-in registers at boot and plugins through sdk.RegisterShippingCarrier, host-adapted to RegisterCarrier - Read-only once boot completes
var (
	carrierMu    sync.RWMutex
	carriers     = make(map[string]CarrierRegistration)
	carrierOrder []string // registration order, for stable iteration
)

var ErrCarrierNotFound = errors.New("shipping: carrier not found")

// What RegisterCarrier records - Slug is stable and used in config paths and shipping_carriers rows, while DisplayName renders in the admin
type CarrierRegistration struct {
	Slug        string
	Carrier     Carrier
	DisplayName string
	// The delivery kinds the carrier quotes - empty means ship_to_address alone. A method that names no kind takes the first
	Kinds []string
}

// DefaultKind is what an untagged method is - mirrors delivery.KindShipToAddress, which shipping doesn't import
const DefaultKind = "ship_to_address"

// DeliveryKinds is what the carrier declares, defaulted
func (r CarrierRegistration) DeliveryKinds() []string {
	if len(r.Kinds) == 0 {
		return []string{DefaultKind}
	}
	return r.Kinds
}

// tag fills an untagged method's kind from the carrier's first
func (r CarrierRegistration) tag(m RatedMethod) RatedMethod {
	if m.Kind == "" {
		m.Kind = r.DeliveryKinds()[0]
	}
	return m
}

// Panics on a duplicate slug or nil impl - a registration bug should refuse to boot, like payment.RegisterProvider
func RegisterCarrier(reg CarrierRegistration) {
	carrierMu.Lock()
	defer carrierMu.Unlock()
	if _, exists := carriers[reg.Slug]; exists {
		panic(fmt.Sprintf("shipping: carrier %q already registered", reg.Slug))
	}
	if reg.Carrier == nil {
		panic(fmt.Sprintf("shipping: carrier %q has nil implementation", reg.Slug))
	}
	if reg.DisplayName == "" {
		reg.DisplayName = reg.Slug
	}
	carriers[reg.Slug] = reg
	carrierOrder = append(carrierOrder, reg.Slug)
}

// ErrCarrierNotFound when the slug is unknown
func GetCarrier(slug string) (CarrierRegistration, error) {
	carrierMu.RLock()
	defer carrierMu.RUnlock()
	reg, ok := carriers[slug]
	if !ok {
		return CarrierRegistration{}, ErrCarrierNotFound
	}
	return reg, nil
}

// Every carrier in registration order, for SyncCarriers seeding and the admin list
func AllCarriers() []CarrierRegistration {
	carrierMu.RLock()
	defer carrierMu.RUnlock()
	out := make([]CarrierRegistration, 0, len(carrierOrder))
	for _, slug := range carrierOrder {
		out = append(out, carriers[slug])
	}
	return out
}

// Drops all carriers - tests only, never production
func ResetForTest() {
	carrierMu.Lock()
	defer carrierMu.Unlock()
	carriers = make(map[string]CarrierRegistration)
	carrierOrder = nil
}
