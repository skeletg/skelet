package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func (r *queryResolver) PulseEngagement(ctx context.Context, channelID string, sinceDays *int32) (*model.PulseEngagement, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(channelID); err != nil {
		return nil, errInvalidID("channelId", err)
	}
	authed, err := signalsCtx(ctx, r.Resolver)
	if err != nil {
		return nil, err
	}
	eng, err := r.SignalsAPI.Pulse.GetEngagement(authed, &signalsv1.GetEngagementRequest{
		ChannelId: channelID, SinceDays: signalsDays(sinceDays),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.PulseEngagement{
		OpensRaw:    eng.GetOpensRaw(),
		OpensHuman:  eng.GetOpensHuman(),
		Clicks:      eng.GetClicks(),
		Conversions: eng.GetConversions(),
		Revenue:     []*model.AttributedRevenue{},
		TopLinks:    []*model.LinkClicks{},
	}
	for _, rv := range eng.GetRevenue() {
		out.Revenue = append(out.Revenue, &model.AttributedRevenue{Currency: rv.GetCurrency(), Cents: int(rv.GetCents()), Orders: rv.GetOrders()})
	}
	for _, l := range eng.GetTopLinks() {
		out.TopLinks = append(out.TopLinks, &model.LinkClicks{URL: l.GetUrl(), Clicks: l.GetClicks()})
	}
	return out, nil
}
