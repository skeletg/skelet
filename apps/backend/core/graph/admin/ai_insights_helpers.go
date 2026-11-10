package admin

import (
	"context"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
)

// reportDays is a report's window: the default when none or nonsense is asked for, never past the longest
func reportDays(days *int32) int32 {
	if days == nil || *days <= 0 {
		return signalsapi.DefaultReportDays
	}
	return min(*days, signalsapi.MaxReportDays)
}

func (r *queryResolver) aiProductNames(ctx context.Context, locale string, products []*signalsv1.AIProduct) (map[uuid.UUID]string, error) {
	ids := make([]uuid.UUID, 0, len(products))
	for _, p := range products {
		if id, err := uuid.Parse(p.GetProductId()); err == nil {
			ids = append(ids, id)
		}
	}
	out := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.Queries.ProductNamesByIDs(ctx, db.ProductNamesByIDsParams{Ids: ids, Locale: locale})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out, nil
}

func aiInsightsOut(days int32, currency string, in *signalsv1.GetAIInsightsResponse, names map[uuid.UUID]string) *model.AIInsights {
	out := &model.AIInsights{Days: days, CurrencyCode: currency, CartWindowDays: signalsapi.AICartWindowDays, OrderWindowDays: signalsapi.AIOrderWindowDays, Chatted: aiCohort(in.GetChatted()), DidNotChat: aiCohort(in.GetDidNotChat()),
		Products: make([]*model.AIRecommendedProduct, 0, len(in.GetProducts())), Agents: make([]*model.AIAgentActivity, 0, len(in.GetAgents())),
		AgentOrders: int32(in.GetAgentOrders()), AgentRevenue: int(in.GetAgentRevenueCents())}
	for _, p := range in.GetProducts() {
		row := &model.AIRecommendedProduct{ProductID: p.GetProductId(), Shown: int32(p.GetShown()), People: int32(p.GetPeople()),
			Carted: int32(p.GetCarted()), Ordered: int32(p.GetOrdered())}
		if id, err := uuid.Parse(p.GetProductId()); err == nil {
			if name, ok := names[id]; ok {
				row.Name = &name
			}
		}
		out.Products = append(out.Products, row)
	}
	for _, a := range in.GetAgents() {
		row := &model.AIAgentActivity{Agent: a.GetAgent(), Calls: int32(a.GetCalls()), Searches: int32(a.GetSearches()),
			EmptySearches: int32(a.GetEmptySearches()), Tools: make([]*model.AIToolCount, 0, len(a.GetTools()))}
		for _, t := range a.GetTools() {
			row.Tools = append(row.Tools, &model.AIToolCount{Tool: t.GetTool(), Calls: int32(t.GetCalls())})
		}
		out.Agents = append(out.Agents, row)
	}
	return out
}

func aiCohort(c *signalsv1.AICohort) *model.AICohort {
	return &model.AICohort{People: int32(c.GetPeople()), Buyers: int32(c.GetBuyers()), Orders: int32(c.GetOrders()),
		Revenue: int(c.GetRevenueCents()), RevenueOrders: int32(c.GetRevenueOrders()), ReturnedOrders: int32(c.GetReturnedOrders())}
}
