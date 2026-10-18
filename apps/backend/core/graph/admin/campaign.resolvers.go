package admin

import (
	"context"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func (r *mutationResolver) CreateEmailCampaign(ctx context.Context, input model.SaveCampaignInput) (*model.EmailCampaign, error) {
	authed, err := r.signalsChannel(ctx, "campaigns.manage", input.ChannelID)
	if err != nil {
		return nil, err
	}
	in, err := campaignInput(input)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Campaigns.CreateCampaign(authed, &signalsv1.CreateCampaignRequest{Campaign: in})
	if err != nil {
		return nil, signalsErr(err)
	}
	return campaignModel(res.GetCampaign()), nil
}

func (r *mutationResolver) UpdateEmailCampaign(ctx context.Context, id string, input model.SaveCampaignInput) (*model.EmailCampaign, error) {
	authed, err := r.signalsScoped(ctx, "campaigns.manage")
	if err != nil {
		return nil, err
	}
	in, err := campaignInput(input)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Campaigns.UpdateCampaign(authed, &signalsv1.UpdateCampaignRequest{Id: id, Campaign: in})
	if err != nil {
		return nil, signalsErr(err)
	}
	return campaignModel(res.GetCampaign()), nil
}

func (r *mutationResolver) DeleteEmailCampaign(ctx context.Context, id string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "campaigns.manage")
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Campaigns.DeleteCampaign(authed, &signalsv1.DeleteCampaignRequest{Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) RestoreEmailCampaign(ctx context.Context, id string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "campaigns.manage")
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Campaigns.RestoreCampaign(authed, &signalsv1.RestoreCampaignRequest{Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) ScheduleEmailCampaign(ctx context.Context, id string, scheduledFor *time.Time) (*model.EmailCampaign, error) {
	authed, err := r.signalsScoped(ctx, "campaigns.manage")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Campaigns.ScheduleCampaign(authed, &signalsv1.ScheduleCampaignRequest{Id: id, ScheduledFor: protoTimeFrom(scheduledFor)})
	if err != nil {
		return nil, signalsErr(err)
	}
	return campaignModel(res.GetCampaign()), nil
}

func (r *mutationResolver) CancelEmailCampaign(ctx context.Context, id string) (*model.EmailCampaign, error) {
	authed, err := r.signalsScoped(ctx, "campaigns.manage")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Campaigns.CancelCampaign(authed, &signalsv1.CancelCampaignRequest{Id: id})
	if err != nil {
		return nil, signalsErr(err)
	}
	return campaignModel(res.GetCampaign()), nil
}

func (r *mutationResolver) SendCampaignTest(ctx context.Context, id string, to string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "campaigns.manage")
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Campaigns.SendCampaignTest(authed, &signalsv1.SendCampaignTestRequest{Id: id, To: to}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *queryResolver) EmailCampaigns(ctx context.Context, channelID string, includeDeleted *bool) ([]*model.EmailCampaign, error) {
	authed, err := r.signalsChannel(ctx, "campaigns.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Campaigns.ListCampaigns(authed, &signalsv1.ListCampaignsRequest{ChannelId: channelID, IncludeDeleted: boolOrFalse(includeDeleted)})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.EmailCampaign, 0, len(res.GetCampaigns()))
	for _, c := range res.GetCampaigns() {
		out = append(out, campaignModel(c))
	}
	return out, nil
}

func (r *queryResolver) EmailCampaign(ctx context.Context, id string) (*model.EmailCampaign, error) {
	authed, err := r.signalsScoped(ctx, "campaigns.view")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Campaigns.GetCampaign(authed, &signalsv1.GetCampaignRequest{Id: id})
	if err != nil {
		if signalsNotFound(err) {
			return nil, nil
		}
		return nil, signalsErr(err)
	}
	return campaignModel(res.GetCampaign()), nil
}

func (r *queryResolver) CampaignVariantStats(ctx context.Context, id string) ([]*model.CampaignVariantStat, error) {
	authed, err := r.signalsScoped(ctx, "campaigns.view")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Pulse.GetCampaignVariantStats(authed, &signalsv1.GetCampaignVariantStatsRequest{CampaignId: id})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.CampaignVariantStat, 0, len(res.GetVariants()))
	for _, st := range res.GetVariants() {
		out = append(out, &model.CampaignVariantStat{Variant: st.GetVariant(), Sent: st.GetSent(), Opened: st.GetOpened(), Clicked: st.GetClicked()})
	}
	return out, nil
}

// The palette is the contract's own list - the suite fills every tag on it
func (r *queryResolver) CampaignMergeTags(ctx context.Context) ([]string, error) {
	if _, err := r.signalsScoped(ctx, "emails.view"); err != nil {
		return nil, err
	}
	return signalsapi.CampaignMergeTags(), nil
}

func (r *queryResolver) CampaignAudienceEstimate(ctx context.Context, channelID string, audienceKind string, audienceID *string) (*model.CampaignAudienceEstimate, error) {
	authed, err := r.signalsChannel(ctx, "campaigns.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Campaigns.EstimateCampaignAudience(authed, &signalsv1.EstimateCampaignAudienceRequest{
		ChannelId: channelID, AudienceKind: audienceKind, AudienceId: audienceID,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return &model.CampaignAudienceEstimate{Total: res.GetTotal(), Eligible: res.GetEligible()}, nil
}

func (r *queryResolver) RenderCampaignPreview(ctx context.Context, channelID string, subject string, blocks interface{}, styles interface{}) (*model.EmailPreview, error) {
	authed, err := r.signalsChannel(ctx, "emails.view", channelID)
	if err != nil {
		return nil, err
	}
	blocksJSON, err := jsonText(blocks)
	if err != nil {
		return nil, errInvalidInput("blocks", err)
	}
	stylesJSON, err := jsonText(styles)
	if err != nil {
		return nil, errInvalidInput("styles", err)
	}
	res, err := r.SignalsAPI.Campaigns.RenderCampaignPreview(authed, &signalsv1.RenderCampaignPreviewRequest{
		ChannelId: channelID, Subject: subject, BlocksJson: blocksJSON, StylesJson: stylesJSON,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return &model.EmailPreview{Subject: res.GetSubject(), HTML: res.GetHtml()}, nil
}
