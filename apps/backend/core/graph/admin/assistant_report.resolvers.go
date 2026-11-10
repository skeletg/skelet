package admin

import (
	"context"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *queryResolver) AssistantReport(ctx context.Context, channelID string, days *int32) (*model.AssistantReport, error) {
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "reports.view", cid, nil); err != nil {
		return nil, err
	}
	rep, err := r.AIUsageSvc.Report(ctx, cid, int(reportDays(days)))
	if err != nil {
		return nil, err
	}
	out := &model.AssistantReport{
		Days: int32(rep.Days), Turns: count32(rep.Turns), FailedTurns: count32(rep.FailedTurns), UnansweredTurns: count32(rep.UnansweredTurns),
		CartTurns: count32(rep.CartTurns), CheckoutTurns: count32(rep.CheckoutTurns), SearchOverviews: count32(rep.Overviews),
		Usage: &model.AssistantUsage{InputTokens: int(rep.Usage.Input), OutputTokens: int(rep.Usage.Output),
			CacheReadTokens: int(rep.Usage.CacheRead), CacheWriteTokens: int(rep.Usage.CacheWrite), EstimatedCostUsd: rep.CostUSD},
		Orders: make([]*model.AssistedOrders, len(rep.Orders)),
	}
	out.Daily = make([]*model.AssistantDay, len(rep.Daily))
	for i, d := range rep.Daily {
		out.Daily[i] = &model.AssistantDay{Day: d.Day.Format(time.DateOnly), Turns: count32(d.Turns), FailedTurns: count32(d.FailedTurns), UnansweredTurns: count32(d.UnansweredTurns),
			CartTurns: count32(d.CartTurns), CheckoutTurns: count32(d.CheckoutTurns), AssistantOrders: d.AssistantOrders, AgentOrders: d.AgentOrders}
	}
	for i, o := range rep.Orders {
		out.Orders[i] = &model.AssistedOrders{CurrencyCode: o.CurrencyCode, OrderCount: o.All, AssistantOrders: o.Assistant,
			AssistantRevenue: int(o.AssistantRevenue), AgentOrders: o.Agent, AgentRevenue: int(o.AgentRevenue)}
	}
	return out, nil
}
