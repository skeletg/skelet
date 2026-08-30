package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/graph/admin"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/graphqlx"
)

// End-to-end over the REAL admin schema - the unit tests around the extension only prove it flips a bool, this proves gqlgen's executor honours that bool for __schema

type gateChecker struct{ grant bool }

func (g gateChecker) HasPermission(context.Context, uuid.UUID, string) (bool, error) {
	return g.grant, nil
}

func introspect(t *testing.T, ctx context.Context, checker auth.PermissionChecker) (string, bool) {
	t.Helper()
	srv := handler.New(admin.NewExecutableSchema(admin.Config{Resolvers: &admin.Resolver{}})) // introspection resolves the schema, never a resolver, so the zero value is enough
	srv.AddTransport(transport.POST{})
	srv.Use(graphqlx.PermittedIntrospection{Checker: checker, Slug: "developer.playground"})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/graphql",
		strings.NewReader(`{"query":"{ __schema { queryType { name } } }"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var body struct {
		Data struct {
			Schema *struct {
				QueryType struct{ Name string } `json:"queryType"`
			} `json:"__schema"`
		} `json:"data"`
		Errors []struct{ Message string } `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	msg := ""
	if len(body.Errors) > 0 {
		msg = body.Errors[0].Message
	}
	return msg, body.Data.Schema != nil && body.Data.Schema.QueryType.Name == "Query"
}

func TestIntrospectionGateBlocksAnonymousOnTheRealSchema(t *testing.T) {
	msg, got := introspect(t, context.Background(), gateChecker{grant: true})
	if got {
		t.Error("anonymous caller introspected the admin schema")
	}
	if !strings.Contains(msg, "introspection disabled") {
		t.Errorf("error was %q, want it to name introspection being disabled", msg)
	}
}

func TestIntrospectionGateAllowsThePermissionOnTheRealSchema(t *testing.T) {
	ctx := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New()})
	msg, got := introspect(t, ctx, gateChecker{grant: true})
	if !got {
		t.Errorf("admin holding developer.playground could not introspect (error %q)", msg)
	}
}

func TestIntrospectionGateBlocksAdminWithoutThePermissionOnTheRealSchema(t *testing.T) {
	ctx := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New()})
	if _, got := introspect(t, ctx, gateChecker{grant: false}); got {
		t.Error("admin lacking developer.playground introspected the admin schema")
	}
}
