package graphqlx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/skeletg/skelet/apps/backend/core/graph/front"
	"github.com/skeletg/skelet/apps/backend/core/internal/graphqlx"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

type captured struct {
	mu     sync.Mutex
	errors []*apperr.Error
	panics []any
}

func (c *captured) CaptureAppError(_ context.Context, ae *apperr.Error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors = append(c.errors, ae)
}

func (c *captured) CapturePanic(_ context.Context, rec any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.panics = append(c.panics, rec)
}

type gqlError struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions"`
}

// Over the real front schema and gqlgen's own extensions, so a gqlgen upgrade that changes how it raises these is caught here
func serveFront(t *testing.T, s graphqlx.Surface, body string) ([]gqlError, *captured) {
	t.Helper()
	rep := &captured{}
	srv := handler.New(front.NewExecutableSchema(front.Config{Resolvers: &front.Resolver{}}))
	srv.AddTransport(transport.POST{})
	srv.Use(extension.AutomaticPersistedQuery{Cache: lru.New[string](10)})
	srv.Use(extension.FixedComplexityLimit(1))
	srv.SetErrorPresenter(graphqlx.Presenter(s, graphqlx.WithReporter(rep)))
	req := httptest.NewRequest(http.MethodPost, "/api/front/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	var out struct {
		Errors []gqlError `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if len(out.Errors) == 0 {
		t.Fatalf("no errors in %s", rec.Body.String())
	}
	return out.Errors, rep
}

// A persisted-query client sends the hash alone first and resends the full query only on this code - presented as INTERNAL it never retried, and every first contact went to Sentry
func TestGqlgenRefusalsReadAsTheCallersMistake(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
	}{
		{"unknown persisted query", `{"extensions":{"persistedQuery":{"version":1,"sha256Hash":"` + strings.Repeat("a", 64) + `"}}}`, "PERSISTED_QUERY_NOT_FOUND"},
		{"over the complexity limit", `{"query":"{ countries { code } currencies { code } }"}`, "COMPLEXITY_LIMIT_EXCEEDED"},
		{"mismatched persisted hash", `{"query":"{ countries { code } }","extensions":{"persistedQuery":{"version":1,"sha256Hash":"` + strings.Repeat("b", 64) + `"}}}`, ""},
		{"unparseable", `{"query":"{ countries {"}`, "GRAPHQL_PARSE_FAILED"},
	} {
		for _, s := range []graphqlx.Surface{graphqlx.SurfaceFront, graphqlx.SurfaceAdmin} {
			t.Run(tc.name+"/"+s.String(), func(t *testing.T) {
				errs, rep := serveFront(t, s, tc.body)
				e := errs[0]
				if e.Extensions["class"] != "VALIDATION" {
					t.Errorf("class = %v, want VALIDATION", e.Extensions["class"])
				}
				if got, _ := e.Extensions["code"].(string); got != tc.code {
					t.Errorf("code = %q, want %q", got, tc.code)
				}
				if e.Message == "internal error" {
					t.Error("the refusal was scrubbed to internal error")
				}
				if len(rep.errors) != 0 {
					t.Errorf("%d reports sent to the aggregator for a caller's mistake", len(rep.errors))
				}
			})
		}
	}
}
