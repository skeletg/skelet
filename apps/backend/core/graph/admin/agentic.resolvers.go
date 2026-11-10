package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *queryResolver) AgenticStatus(ctx context.Context, channelID string) ([]*model.AgenticStatus, error) {
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
	return r.agenticStatus(ctx, ch)
}
