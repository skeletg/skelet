package wiring

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/agenttools"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/mcpserver"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
)

// A field left out here is a nil service the first tool call panics on
func TestAgentToolsGetEveryService(t *testing.T) {
	var s Svcs
	for _, f := range reflect.ValueOf(&s).Elem().Fields() {
		if f.Kind() == reflect.Pointer && f.CanSet() {
			f.Set(reflect.New(f.Type().Elem()))
		}
	}
	for field, f := range reflect.ValueOf(agentToolDeps(Infra{Queries: &db.Queries{}}, s)).Fields() {
		if (f.Kind() == reflect.Pointer || f.Kind() == reflect.Interface) && f.IsNil() {
			t.Errorf("agenttools.Deps.%s is never wired", field.Name)
		}
	}
}

func TestEveryAgentToolRegistersWithTheMCPServer(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("the MCP server refused a tool definition: %v", rec)
		}
	}()
	if len(agenttools.Definitions()) == 0 {
		t.Fatal("the registry is empty")
	}
	resolve := mcpserver.Admin(func(context.Context, uuid.UUID, int) mcpserver.ToolSet { return nil }, nil)
	mcpserver.NewHandler(mcpserver.Config{Tools: agenttools.Definitions(), Resolve: resolve})
	mcpserver.NewHandler(mcpserver.Config{Tools: shoptools.Definitions(), Resolve: resolve})
}
