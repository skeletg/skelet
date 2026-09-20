package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *queryResolver) SignalsInsights(ctx context.Context, channelID string, sinceDays *int32) ([]*model.SignalsInsight, error) {
	// Deterministic scan of notable changes, no LLM - and digestPermsFor treats a missing admin as full bypass (see LoadPermittedChannels), so without this guard an anonymous caller reads every channel's Signals analytics
	if _, err := auth.RequireAuth(ctx); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channel id", nil)
	}
	perms := digestPermsFor(ctx, r.Resolver, cid)
	if !perms.any() {
		return []*model.SignalsInsight{}, nil
	}
	days := 30
	if sinceDays != nil && *sinceDays > 0 {
		days = int(*sinceDays)
	}
	return buildInsights(ctx, r.Resolver, cid, days, perms), nil
}
