package shoptools_test

import (
	"testing"

	"github.com/vektah/gqlparser/v2"

	graphfront "github.com/skeletg/skelet/apps/backend/core/graph/front"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
)

// The tools' documents are Go strings, so no codegen checks them - a renamed front field would only show up as a failing agent
func TestEveryDocumentValidatesAgainstTheFrontSchema(t *testing.T) {
	schema := graphfront.NewExecutableSchema(graphfront.Config{}).Schema()
	docs := shoptools.Documents()
	if len(docs) < 10 {
		t.Fatalf("only %d documents - the list fell out of step with the tools", len(docs))
	}
	for name, doc := range docs {
		if _, errs := gqlparser.LoadQueryWithRules(schema, doc, nil); len(errs) > 0 {
			t.Errorf("%s: %v", name, errs)
		}
	}
}
