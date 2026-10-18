package signalsgrpc

// The campaign composer's reads and writes, relayed by core with the caller's grants

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
)

type campaignServer struct {
	signalsv1.UnimplementedCampaignServiceServer
	svc *campaign.Service
}

func (s *campaignServer) ListCampaigns(ctx context.Context, req *signalsv1.ListCampaignsRequest) (*signalsv1.ListCampaignsResponse, error) {
	cid, err := channelScopeOrSystem(ctx, "campaigns.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	list, err := s.svc.List(ctx, cid, req.GetIncludeDeleted())
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.Campaign, 0, len(list))
	for _, c := range list {
		out = append(out, campaignProto(c))
	}
	return &signalsv1.ListCampaignsResponse{Campaigns: out}, nil
}

func (s *campaignServer) GetCampaign(ctx context.Context, req *signalsv1.GetCampaignRequest) (*signalsv1.GetCampaignResponse, error) {
	c, err := s.load(ctx, "campaigns.view", req.GetId())
	if err != nil {
		return nil, err
	}
	return &signalsv1.GetCampaignResponse{Campaign: campaignProto(c)}, nil
}

func (s *campaignServer) CreateCampaign(ctx context.Context, req *signalsv1.CreateCampaignRequest) (*signalsv1.CreateCampaignResponse, error) {
	in, err := campaignInput(req.GetCampaign())
	if err != nil {
		return nil, err
	}
	if _, err := channelScopeOrSystem(ctx, "campaigns.manage", req.GetCampaign().GetChannelId()); err != nil {
		return nil, err
	}
	c, err := s.svc.Create(ctx, in)
	if err != nil {
		return nil, campaignErr(err)
	}
	return &signalsv1.CreateCampaignResponse{Campaign: campaignProto(c)}, nil
}

func (s *campaignServer) UpdateCampaign(ctx context.Context, req *signalsv1.UpdateCampaignRequest) (*signalsv1.UpdateCampaignResponse, error) {
	existing, err := s.load(ctx, "campaigns.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	in, err := campaignInput(req.GetCampaign())
	if err != nil {
		return nil, err
	}
	in.ChannelID = existing.ChannelID
	c, err := s.svc.Update(ctx, existing.ID, in)
	if err != nil {
		return nil, campaignErr(err)
	}
	return &signalsv1.UpdateCampaignResponse{Campaign: campaignProto(c)}, nil
}

func (s *campaignServer) DeleteCampaign(ctx context.Context, req *signalsv1.DeleteCampaignRequest) (*signalsv1.DeleteCampaignResponse, error) {
	c, err := s.load(ctx, "campaigns.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.SoftDelete(ctx, c.ID); err != nil {
		return nil, err
	}
	return &signalsv1.DeleteCampaignResponse{}, nil
}

func (s *campaignServer) RestoreCampaign(ctx context.Context, req *signalsv1.RestoreCampaignRequest) (*signalsv1.RestoreCampaignResponse, error) {
	c, err := s.load(ctx, "campaigns.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Restore(ctx, c.ID); err != nil {
		return nil, err
	}
	return &signalsv1.RestoreCampaignResponse{}, nil
}

func (s *campaignServer) ScheduleCampaign(ctx context.Context, req *signalsv1.ScheduleCampaignRequest) (*signalsv1.ScheduleCampaignResponse, error) {
	c, err := s.load(ctx, "campaigns.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Schedule(ctx, c.ID, timeFromTS(req.GetScheduledFor())); err != nil {
		return nil, campaignErr(err)
	}
	c, err = s.reload(ctx, c)
	if err != nil {
		return nil, err
	}
	return &signalsv1.ScheduleCampaignResponse{Campaign: campaignProto(c)}, nil
}

func (s *campaignServer) CancelCampaign(ctx context.Context, req *signalsv1.CancelCampaignRequest) (*signalsv1.CancelCampaignResponse, error) {
	c, err := s.load(ctx, "campaigns.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Cancel(ctx, c.ID); err != nil {
		return nil, campaignErr(err)
	}
	c, err = s.reload(ctx, c)
	if err != nil {
		return nil, err
	}
	return &signalsv1.CancelCampaignResponse{Campaign: campaignProto(c)}, nil
}

func (s *campaignServer) SendCampaignTest(ctx context.Context, req *signalsv1.SendCampaignTestRequest) (*signalsv1.SendCampaignTestResponse, error) {
	c, err := s.load(ctx, "campaigns.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.SendTest(ctx, c.ID, req.GetTo()); err != nil {
		return nil, campaignErr(err)
	}
	return &signalsv1.SendCampaignTestResponse{}, nil
}

func (s *campaignServer) EstimateCampaignAudience(ctx context.Context, req *signalsv1.EstimateCampaignAudienceRequest) (*signalsv1.EstimateCampaignAudienceResponse, error) {
	cid, err := channelScopeOrSystem(ctx, "campaigns.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	aid, err := optionalIDFrom(req.AudienceId, "audience_id")
	if err != nil {
		return nil, err
	}
	total, eligible, err := s.svc.EstimateAudience(ctx, cid, req.GetAudienceKind(), aid)
	if err != nil {
		return nil, campaignErr(err)
	}
	return &signalsv1.EstimateCampaignAudienceResponse{Total: int32(total), Eligible: int32(eligible)}, nil
}

func (s *campaignServer) RenderCampaignPreview(ctx context.Context, req *signalsv1.RenderCampaignPreviewRequest) (*signalsv1.RenderCampaignPreviewResponse, error) {
	cid, err := channelScopeOrSystem(ctx, "emails.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	blocks, err := blocksFromJSON(req.GetBlocksJson())
	if err != nil {
		return nil, err
	}
	styles, err := stylesFromJSON(req.GetStylesJson())
	if err != nil {
		return nil, err
	}
	subject, html, err := s.svc.Preview(ctx, cid, req.GetSubject(), blocks, styles)
	if err != nil {
		return nil, campaignErr(err)
	}
	return &signalsv1.RenderCampaignPreviewResponse{Subject: subject, Html: html}, nil
}

func (s *campaignServer) load(ctx context.Context, perm, rawID string) (campaign.Campaign, error) {
	if err := requirePermissionOrSystem(ctx, perm); err != nil {
		return campaign.Campaign{}, err
	}
	id, err := parseID("id", rawID)
	if err != nil {
		return campaign.Campaign{}, err
	}
	c, ok, err := s.svc.Get(ctx, id)
	if err != nil {
		return campaign.Campaign{}, err
	}
	if !ok {
		return campaign.Campaign{}, notFound("campaign")
	}
	if err := requireChannelOrSystem(ctx, perm, c.ChannelID); err != nil {
		return campaign.Campaign{}, err
	}
	return c, nil
}

// A mutation answers with the campaign as it now stands
func (s *campaignServer) reload(ctx context.Context, c campaign.Campaign) (campaign.Campaign, error) {
	got, ok, err := s.svc.Get(ctx, c.ID)
	if err != nil {
		return campaign.Campaign{}, err
	}
	if !ok {
		return campaign.Campaign{}, notFound("campaign")
	}
	return got, nil
}

func campaignInput(in *signalsv1.CampaignInput) (campaign.Input, error) {
	cid, err := parseID("channel_id", in.GetChannelId())
	if err != nil {
		return campaign.Input{}, err
	}
	blocks, err := blocksFromJSON(in.GetBlocksJson())
	if err != nil {
		return campaign.Input{}, err
	}
	styles, err := stylesFromJSON(in.GetStylesJson())
	if err != nil {
		return campaign.Input{}, err
	}
	aid, err := optionalIDFrom(in.AudienceId, "audience_id")
	if err != nil {
		return campaign.Input{}, err
	}
	eid, err := optionalIDFrom(in.EmailId, "email_id")
	if err != nil {
		return campaign.Input{}, err
	}
	return campaign.Input{
		ChannelID: cid, Name: in.GetName(), EmailID: eid, Subject: in.GetSubject(), Blocks: blocks, Styles: styles,
		AudienceKind: in.GetAudienceKind(), AudienceID: aid,
		ABEnabled: in.GetAbEnabled(), ABSubjectB: in.GetAbSubjectB(), ABTestPercent: int(in.GetAbTestPercent()),
		ABWinnerMetric: in.GetAbWinnerMetric(), ABTestMinutes: int(in.GetAbTestMinutes()),
	}, nil
}

func campaignProto(c campaign.Campaign) *signalsv1.Campaign {
	return &signalsv1.Campaign{
		Id: c.ID.String(), ChannelId: c.ChannelID.String(), Name: c.Name, EmailId: optionalIDString(c.EmailID),
		Subject: c.Subject, BlocksJson: blocksJSON(c.Blocks), StylesJson: stylesJSON(c.Styles),
		AudienceKind: c.AudienceKind, AudienceId: optionalIDString(c.AudienceID), Status: c.Status,
		ScheduledFor: tsPtr(c.ScheduledFor), SentAt: tsPtr(c.SentAt),
		TotalRecipients: int32(c.TotalRecipients), SentCount: int32(c.SentCount), FailedCount: int32(c.FailedCount),
		SkippedCount: int32(c.SkippedCount), OpenedCount: int32(c.OpenedCount), ClickedCount: int32(c.ClickedCount),
		AbEnabled: c.ABEnabled, AbSubjectB: c.ABSubjectB, AbTestPercent: int32(c.ABTestPercent), AbWinnerMetric: c.ABWinnerMetric,
		AbTestMinutes: int32(c.ABTestMinutes), AbWinner: c.ABWinner, AbTestStartedAt: tsPtr(c.ABTestStartedAt),
		CreatedAt: ts(c.CreatedAt), UpdatedAt: ts(c.UpdatedAt), DeletedAt: tsPtr(c.DeletedAt),
	}
}

// The campaign service's sentinels as the codes core's resolvers render - anything else is the suite's own failure
func campaignErr(err error) error {
	switch {
	case errors.Is(err, campaign.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, campaign.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, campaign.ErrWrongState):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, campaign.ErrSendFailed):
		return status.Error(codes.Unavailable, err.Error())
	}
	return err
}
