package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

func (r *Resolver) searchMerchChannel(ctx context.Context, channelID string) (uuid.UUID, error) {
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return uuid.UUID{}, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "searchmerch.manage", cid, nil); err != nil {
		return uuid.UUID{}, err
	}
	return cid, nil
}
