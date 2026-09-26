// Package contract carries the published remote-provider wire contract - a JSON Schema and an example request per kind, generated from the adapter structs by cmd/providerschema
// Baked into the binary so bin/provider-check has the examples wherever it runs, and so a stale checkout cannot post a payload the running core no longer accepts
package contract

import (
	"embed"
	"fmt"
)

// The wire contract's own version, sent on every invocation as X-Provider-Contract-Version and in the envelope, so a provider can log what it was built against and detect drift - 0.x while the contract is unfrozen: a breaking change (removed/renamed field, changed type, new required expectation) bumps the minor, an additive one (new kind, new optional field) the patch contract_lock_test.go refuses schema drift without a bump here
const Version = "0.2.1"

//go:embed *.json
var files embed.FS

// ExampleRequest returns the generated example payload for a kind - the body of the
// envelope's `payload` field, not the whole envelope
func ExampleRequest(kind string) ([]byte, error) {
	b, err := files.ReadFile(kind + ".request.json")
	if err != nil {
		return nil, fmt.Errorf("no generated example for kind %q (run bin/codegen)", kind)
	}
	return b, nil
}
