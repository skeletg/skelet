package main

import (
	"os"
	"strings"
	"testing"
)

// Every field of the SDK's event shapes has a TypeScript type, and the committed file is what render writes - so a field
// added in Go without a mapping fails here rather than in a storefront's build
func TestTheContractRendersAndIsCommitted(t *testing.T) {
	out, err := render()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"view_item"`, "item_id: string;", "export type Ecommerce", "CONSENT_PATTERN = /^"} {
		if !strings.Contains(out, want) {
			t.Errorf("the contract lacks %s", want)
		}
	}
	committed, err := os.ReadFile("../../../../.." + target)
	if err != nil {
		t.Skip("no storefront-core in this checkout")
	}
	if string(committed) != out {
		t.Error("contract.gen.ts is stale - run skelet gen")
	}
}
