// Package conditions ships the built-in promotion-condition types - each file adds itself from init(), and Register installs the set into a registry, which wiring does when it builds the promotion service
package conditions

import (
	"encoding/json"
	"fmt"
)

func unmarshalParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode params: %w", err)
	}
	return nil
}
