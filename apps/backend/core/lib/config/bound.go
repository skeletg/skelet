package config

import (
	"strings"
	"time"
)

// A key saved this long before its address still counts as entered for it - one save that writes the key, then the address
const BoundSecretGrace = 10 * time.Second

// BoundSecret is the key when it was entered for the address in use. One saved before the address last moved was meant
// for somewhere else, so it isn't handed to the new address - the built-in address takes any key
func BoundSecret(key, address Entry) string {
	if key.Value == "" || address.IsDefault || strings.TrimSpace(address.Value) == "" {
		return key.Value
	}
	if key.UpdatedAt.Before(address.UpdatedAt.Add(-BoundSecretGrace)) {
		return ""
	}
	return key.Value
}
