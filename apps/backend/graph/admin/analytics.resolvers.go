package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

func (r *queryResolver) PulseEngagement(ctx context.Context, channelID string, sinceDays *int32) (*model.PulseEngagement, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	days := int32(30)
	if sinceDays != nil && *sinceDays > 0 {
		days = *sinceDays
	}
	eng, err := r.AnalyticsSvc.Engagement(ctx, cid, time.Now().AddDate(0, 0, -int(days)))
	if err != nil {
		return nil, err
	}
	out := &model.PulseEngagement{
		OpensRaw:    int32(eng.OpensRaw),
		OpensHuman:  int32(eng.OpensHuman),
		Clicks:      int32(eng.Clicks),
		Conversions: int32(eng.Conversions),
		Revenue:     []*model.AttributedRevenue{},
		TopLinks:    []*model.LinkClicks{},
	}
	for _, rv := range eng.Revenue {
		out.Revenue = append(out.Revenue, &model.AttributedRevenue{Currency: rv.Currency, Cents: int(rv.Cents), Orders: int32(rv.Orders)})
	}
	for _, l := range eng.TopLinks {
		out.TopLinks = append(out.TopLinks, &model.LinkClicks{URL: l.URL, Clicks: int32(l.Clicks)})
	}
	return out, nil
}
