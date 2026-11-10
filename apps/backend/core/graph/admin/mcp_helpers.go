package admin

import (
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

// The switches are environment, read once at boot by the same EnvBool calls that mount the endpoints
type MCPEndpoints struct {
	Admin, Storefront bool
	AdminPath         string
}

const storefrontMCPPath = "/api/mcp"

func storefrontMCPURL(publicURL string) *string {
	base := strings.TrimRight(strings.TrimSpace(publicURL), "/")
	if base == "" {
		return nil
	}
	u := base + storefrontMCPPath
	return &u
}

func mcpTools(defs []ai.Tool) []*model.McpTool {
	out := make([]*model.McpTool, len(defs))
	for i, d := range defs {
		out[i] = &model.McpTool{Name: d.Name, Description: d.Description}
	}
	return out
}
