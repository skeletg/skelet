package shoptools_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/vektah/gqlparser/v2"

	graphfront "github.com/skeletg/skelet/apps/backend/core/graph/front"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
)

// The tools' documents are Go strings, so no codegen checks them - a renamed front field would only show up as a failing agent
func TestEveryDocumentValidatesAgainstTheFrontSchema(t *testing.T) {
	schema := graphfront.NewExecutableSchema(graphfront.Config{}).Schema()
	docs := shoptools.Documents()
	registered := slices.Collect(maps.Values(docs))
	for _, d := range shoptools.SentDocuments {
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
