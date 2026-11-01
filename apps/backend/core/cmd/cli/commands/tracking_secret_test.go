package commands

import (
	"strings"
	"testing"
)

func TestTrackingSecretInputPreservesKeysAndBoundsInput(t *testing.T) {
	const key = "  secret with spaces\nprivate-key-line\n"
	if got, err := readTrackingSecret(strings.NewReader(key)); err != nil || got != key {
		t.Fatalf("secret bytes changed: %v", err)
	}
	if _, err := readTrackingSecret(strings.NewReader(strings.Repeat("x", (64<<10)+1))); err == nil {
		t.Fatal("unbounded secret accepted")
	}
}
