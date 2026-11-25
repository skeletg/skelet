package config

import (
	"fmt"
	"strconv"
	"strings"
)

// intWithin refuses a whole number outside lo..hi - a Definition's Check for a count or a percentage
func intWithin(lo, hi int64) func(string) error {
	return func(value string) error {
		n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || n < lo || n > hi {
			return fmt.Errorf("must be a whole number from %d to %d", lo, hi)
		}
		return nil
	}
}
