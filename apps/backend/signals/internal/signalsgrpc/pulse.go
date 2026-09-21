package signalsgrpc

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
)

type pulseServer struct {
	signalsv1.UnimplementedPulseServiceServer
	rec    *analytics.Recorder
	sends  *sendlog.Store
	roster rosterChannels // the customer's own channel, for the per-customer read below
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

// Gated on customers.view rather than emails.view - this hangs off a customer's profile, and an operator who may see the customer may see what was sent to them
// A flow's email_activity condition asks this as core itself, the admin profile with a grant
func (s *pulseServer) GetCustomerEngagement(ctx context.Context, req *signalsv1.GetCustomerEngagementRequest) (*signalsv1.GetCustomerEngagementResponse, error) {
	cid, err := parseID("customer_id", req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	if s.roster == nil {
		return nil, status.Error(codes.FailedPrecondition, "the customer roster is not configured")
	}
	channelID, err := s.roster.ChannelForCustomer(ctx, cid)
	if err != nil {
		return nil, status.Error(codes.NotFound, "customer not found")
	}
	if err := requireChannelOrSystem(ctx, "customers.view", channelID); err != nil {
		return nil, err
	}
	e, err := s.rec.CustomerEmailEngagement(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := &signalsv1.GetCustomerEngagementResponse{
		Sent: int32(e.Sent), Opened: int32(e.Opened), Clicked: int32(e.Clicked),
	}
	if e.LastSentAt != nil {
		sent := e.LastSentAt.Format(time.RFC3339)
		out.LastSentAt = &sent
	}
	if e.LastOpenAt != nil {
		open := e.LastOpenAt.Format(time.RFC3339)
		out.LastOpenAt = &open
	}
	return out, nil
}

func revenueProto(rows []analytics.RevenueByCurrency) []*signalsv1.AttributedRevenue {
	out := make([]*signalsv1.AttributedRevenue, 0, len(rows))
	for _, r := range rows {
		out = append(out, &signalsv1.AttributedRevenue{Currency: r.Currency, Cents: r.Cents, Orders: int32(r.Orders)})
	}
	return out
}

// The A/B results view asks with a grant, the winner decision as core itself
func (s *pulseServer) GetCampaignVariantStats(ctx context.Context, req *signalsv1.GetCampaignVariantStatsRequest) (*signalsv1.GetCampaignVariantStatsResponse, error) {
	if err := requirePermissionOrSystem(ctx, "campaigns.view"); err != nil { // the A/B results view is the campaign composer's
		return nil, err
	}
	id, err := parseID("campaign_id", req.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if s.sends == nil {
		return &signalsv1.GetCampaignVariantStatsResponse{}, nil
	}
	stats, err := s.sends.VariantStats(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.VariantStat, 0, len(stats))
	for _, st := range stats {
		out = append(out, &signalsv1.VariantStat{Variant: st.Variant, Sent: int32(st.Sent), Opened: int32(st.Opened), Clicked: int32(st.Clicked)})
	}
	return &signalsv1.GetCampaignVariantStatsResponse{Variants: out}, nil
}
