package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/agenttools"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func (r *queryResolver) McpOverview(ctx context.Context, channelID string) (*model.McpOverview, error) {
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "config.view", cid, nil); err != nil {
		return nil, err
	}
	ch, err := r.ChannelSvc.GetByID(ctx, cid)
	if err != nil {
		return nil, err
	}
	enabled, err := r.ConfigSvc.GetBool(ctx, shoptools.MCPEnabledPath, config.Scope{Kind: config.ScopeChannel, ChannelID: &cid})
	if err != nil {
		return nil, err
	}
	return &model.McpOverview{
		Storefront: &model.StorefrontMcp{Available: r.MCP.Storefront, Enabled: enabled, URL: storefrontMCPURL(ch.PublicURL), Tools: mcpTools(shoptools.Definitions())},
		Admin:      &model.AdminMcp{Available: r.MCP.Admin, Path: r.MCP.AdminPath, Tools: mcpTools(agenttools.Definitions())},
	}, nil
}
