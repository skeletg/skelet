// Package effects holds the built-in tax effect types - each file adds itself from init(), and Register installs the set into a registry, which wiring does when it builds the tax service
package effects

import (
	"encoding/json"
	"fmt"
)

// Integer division truncates, which under-collects up to a cent per line per rule and always in the customer's favour - both operands are non-negative here, so a plain +d/2 is round-half-up
func divRoundHalfUp(n, d int64) int64 {
	if d == 0 {
		return 0
	}
	return (n + d/2) / d
}

func unmarshalParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode params: %w", err)
	}
	return nil
}
