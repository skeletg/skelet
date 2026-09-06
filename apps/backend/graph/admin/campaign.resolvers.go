package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/campaign"
)

func (r *mutationResolver) CreateEmailCampaign(ctx context.Context, input model.SaveCampaignInput) (*model.EmailCampaign, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.manage"); err != nil {
		return nil, err
	}
	in, err := campaignInput(input)
	if err != nil {
		return nil, err
	}
	c, err := r.CampaignSvc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return toCampaignModel(c), nil
}

func (r *mutationResolver) UpdateEmailCampaign(ctx context.Context, id string, input model.SaveCampaignInput) (*model.EmailCampaign, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.manage"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	in, err := campaignInput(input)
	if err != nil {
		return nil, err
	}
	c, err := r.CampaignSvc.Update(ctx, cid, in)
	if err != nil {
		return nil, err
	}
	return toCampaignModel(c), nil
}

func (r *mutationResolver) DeleteEmailCampaign(ctx context.Context, id string) (bool, error) {
	if err := r.requireSignals(); err != nil {
		return false, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.manage"); err != nil {
		return false, err
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.CampaignSvc.SoftDelete(ctx, cid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) RestoreEmailCampaign(ctx context.Context, id string) (bool, error) {
	if err := r.requireSignals(); err != nil {
		return false, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.manage"); err != nil {
		return false, err
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.CampaignSvc.Restore(ctx, cid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) ScheduleEmailCampaign(ctx context.Context, id string, scheduledFor *time.Time) (*model.EmailCampaign, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.manage"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	if err := r.CampaignSvc.Schedule(ctx, cid, scheduledFor); err != nil {
		return nil, err
	}
	return r.loadCampaign(ctx, cid)
}

func (r *mutationResolver) CancelEmailCampaign(ctx context.Context, id string) (*model.EmailCampaign, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.manage"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	if err := r.CampaignSvc.Cancel(ctx, cid); err != nil {
		return nil, err
	}
	return r.loadCampaign(ctx, cid)
}

func (r *mutationResolver) SendCampaignTest(ctx context.Context, id string, to string) (bool, error) {
	if err := r.requireSignals(); err != nil {
		return false, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.manage"); err != nil {
		return false, err
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.CampaignSvc.SendTest(ctx, cid, to); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) EmailCampaigns(ctx context.Context, channelID string, includeDeleted *bool) ([]*model.EmailCampaign, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	list, err := r.CampaignSvc.List(ctx, cid, boolOrFalse(includeDeleted))
	if err != nil {
		return nil, err
	}
	out := make([]*model.EmailCampaign, len(list))
	for i, c := range list {
		out[i] = toCampaignModel(c)
	}
	return out, nil
}

func (r *queryResolver) EmailCampaign(ctx context.Context, id string) (*model.EmailCampaign, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	c, ok, err := r.CampaignSvc.Get(ctx, cid)
	if err != nil || !ok {
		return nil, err
	}
	return toCampaignModel(c), nil
}

func (r *queryResolver) CampaignVariantStats(ctx context.Context, id string) ([]*model.CampaignVariantStat, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	stats, err := r.CampaignSvc.VariantStats(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CampaignVariantStat, 0, len(stats))
	for _, st := range stats {
		out = append(out, &model.CampaignVariantStat{Variant: st.Variant, Sent: int32(st.Sent), Opened: int32(st.Opened), Clicked: int32(st.Clicked)})
	}
	return out, nil
}

func (r *queryResolver) CampaignMergeTags(ctx context.Context) ([]string, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.view"); err != nil {
		return nil, err
	}
	return campaign.CampaignMergeTags(), nil
}

func (r *queryResolver) CampaignAudienceEstimate(ctx context.Context, channelID string, audienceKind string, audienceID *string) (*model.CampaignAudienceEstimate, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	aid, err := optionalUUID(audienceID)
	if err != nil {
		return nil, errInvalidID("audienceId", err)
	}
	total, eligible, err := r.CampaignSvc.EstimateAudience(ctx, cid, audienceKind, aid)
	if err != nil {
		return nil, err
	}
	return &model.CampaignAudienceEstimate{Total: int32(total), Eligible: int32(eligible)}, nil
}

func (r *queryResolver) RenderCampaignPreview(ctx context.Context, channelID string, subject string, blocks interface{}, styles interface{}) (*model.EmailPreview, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "campaigns.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	bs, err := blocksFromJSON(blocks)
	if err != nil {
		return nil, err
	}
	st, err := stylesFromJSON(styles)
	if err != nil {
		return nil, err
	}
	subjectOut, html, err := r.CampaignSvc.Preview(ctx, cid, subject, bs, st)
	if err != nil {
		return nil, err
	}
	return &model.EmailPreview{Subject: subjectOut, HTML: html}, nil
}
