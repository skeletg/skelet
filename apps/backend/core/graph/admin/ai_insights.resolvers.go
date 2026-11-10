package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func (r *queryResolver) AiInsights(ctx context.Context, channelID string, days *int32) (*model.AIInsights, error) {
	authed, err := r.signalsChannel(ctx, "assistant.view", channelID)
	if err != nil {
		return nil, err
	}
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	ch, err := r.ChannelSvc.GetByID(ctx, cid)
	if err != nil {
		return nil, err
	}
	window := reportDays(days)
	in, err := r.SignalsAPI.Assistant.GetAIInsights(authed, &signalsv1.GetAIInsightsRequest{ChannelId: channelID, SinceDays: window, Currency: ch.CurrencyCode})
	if err != nil {
		return nil, signalsErr(err)
	}
	names, err := r.aiProductNames(ctx, ch.DefaultLocale, in.GetProducts())
	if err != nil {
		return nil, err
	}
	return aiInsightsOut(window, ch.CurrencyCode, in, names), nil
}
