package signalsgrpc

import (
	"context"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

type pulseServer struct {
	signalsv1.UnimplementedPulseServiceServer
	rec *analytics.Recorder
}

func (s *pulseServer) GetEngagement(ctx context.Context, req *signalsv1.GetEngagementRequest) (*signalsv1.GetEngagementResponse, error) {
	if err := requirePermission(ctx, "emails.view"); err != nil {
		return nil, err
	}
	cid, err := parseID("channel_id", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	days := windowDays(req.GetSinceDays(), 30)
	e, err := s.rec.Engagement(ctx, cid, time.Now().AddDate(0, 0, -int(days)))
	if err != nil {
		return nil, err
	}
	links := make([]*signalsv1.LinkClicks, 0, len(e.TopLinks))
	for _, l := range e.TopLinks {
		links = append(links, &signalsv1.LinkClicks{Url: l.URL, Clicks: int32(l.Clicks)})
	}
	return &signalsv1.GetEngagementResponse{
		OpensRaw: int32(e.OpensRaw), OpensHuman: int32(e.OpensHuman),
		Clicks: int32(e.Clicks), Conversions: int32(e.Conversions),
		Revenue: revenueProto(e.Revenue), TopLinks: links,
	}, nil
}

func revenueProto(rows []analytics.RevenueByCurrency) []*signalsv1.AttributedRevenue {
	out := make([]*signalsv1.AttributedRevenue, 0, len(rows))
	for _, r := range rows {
		out = append(out, &signalsv1.AttributedRevenue{Currency: r.Currency, Cents: r.Cents, Orders: int32(r.Orders)})
	}
	return out
}
