package shoptools

import (
	"maps"
	"slices"
	"testing"

	"github.com/vektah/gqlparser/v2"

	graphfront "github.com/skeletg/skelet/apps/backend/core/graph/front"
)

// The tools' documents are Go strings, so no codegen checks them - a renamed front field would only show up as a failing agent
func TestEveryDocumentValidatesAgainstTheFrontSchema(t *testing.T) {
	schema := graphfront.NewExecutableSchema(graphfront.Config{}).Schema()
	docs := allDocuments()
	registered := slices.Collect(maps.Values(docs))
	for _, d := range []string{searchDoc, productByIDDoc, cartDoc, shippingDoc, pagesDoc, couponDoc} {
		if !slices.Contains(registered, d) {
			t.Fatal("a document the tools send isn't registered, so it would go unchecked")
		}
	}
	for name, doc := range docs {
		if _, errs := gqlparser.LoadQueryWithRules(schema, doc, nil); len(errs) > 0 {
			t.Errorf("%s: %v", name, errs)
		}
	}
}
