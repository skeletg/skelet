// Package contract carries the published remote-provider wire contract - a JSON Schema and
// an example request per kind, generated from the adapter structs by cmd/providerschema
// Baked into the binary so bin/provider-check has the examples wherever it runs, and so a
// stale checkout can't post a payload the running core no longer accepts
package contract

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// The wire contract's own version, sent on every invocation as X-Provider-Contract-Version and
// in the envelope, so a provider can log what it was built against and detect drift - 0.x while
// the contract is unfrozen: a breaking change (removed/renamed field, changed type, new required
// expectation) bumps the minor, an additive one (new kind, new optional field) the patch
// contract_lock_test.go refuses schema drift without a bump here
const Version = "0.1.0"

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

// Schema returns the generated JSON Schema for one operation - `kind` for the primary
// operation, `kind.operation` for a secondary one such as search.browse
func Schema(name string) ([]byte, error) {
	b, err := files.ReadFile(name + ".schema.json")
	if err != nil {
		return nil, fmt.Errorf("no generated schema for %q (run bin/codegen)", name)
	}
	return b, nil
}

// Operations lists every generated operation name, primary and secondary
func Operations() []string {
	entries, err := fs.Glob(files, "*.schema.json")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e, ".schema.json"))
	}
	return out
}
