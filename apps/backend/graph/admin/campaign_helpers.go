package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/campaign"
)

// loadCampaign re-fetches a campaign after a mutation (schedule/cancel) so the
// resolver returns its current state.
func (r *Resolver) loadCampaign(ctx context.Context, id uuid.UUID) (*model.EmailCampaign, error) {
	c, ok, err := r.CampaignSvc.Get(ctx, id)
	if err != nil || !ok {
		return nil, err
	}
	return toCampaignModel(c), nil
}

// campaignInput parses a SaveCampaignInput into the domain Input, reusing the
// email-template JSON helpers for blocks/styles.
func campaignInput(in model.SaveCampaignInput) (campaign.Input, error) {
	cid, err := uuid.Parse(in.ChannelID)
	if err != nil {
		return campaign.Input{}, errInvalidID("channelId", err)
	}
	blocks, err := blocksFromJSON(in.Blocks)
	if err != nil {
		return campaign.Input{}, err
	}
	styles, err := stylesFromJSON(in.Styles)
	if err != nil {
		return campaign.Input{}, err
	}
	aid, err := optionalUUID(in.AudienceID)
	if err != nil {
		return campaign.Input{}, errInvalidID("audienceId", err)
	}
	eid, err := optionalUUID(in.EmailID)
	if err != nil {
		return campaign.Input{}, errInvalidID("emailId", err)
	}
	return campaign.Input{
		ChannelID: cid, Name: in.Name, EmailID: eid, Subject: in.Subject, Blocks: blocks, Styles: styles,
		AudienceKind: in.AudienceKind, AudienceID: aid,
		ABEnabled:      in.AbEnabled != nil && *in.AbEnabled,
		ABSubjectB:     ptrStr(in.AbSubjectB),
		ABTestPercent:  ptrInt(in.AbTestPercent),
		ABWinnerMetric: ptrStr(in.AbWinnerMetric),
		ABTestMinutes:  ptrInt(in.AbTestMinutes),
	}, nil
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptrInt(p *int32) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

func toCampaignModel(c campaign.Campaign) *model.EmailCampaign {
	var aid *string
	if c.AudienceID != nil {
		s := c.AudienceID.String()
		aid = &s
	}
	var eid *string
	if c.EmailID != nil {
		s := c.EmailID.String()
		eid = &s
	}
	return &model.EmailCampaign{
		ID:              c.ID.String(),
		ChannelID:       c.ChannelID.String(),
		Name:            c.Name,
		EmailID:         eid,
		Subject:         c.Subject,
		Blocks:          blocksOrEmpty(c.Blocks),
		Styles:          c.Styles,
		AudienceKind:    c.AudienceKind,
		AudienceID:      aid,
		Status:          c.Status,
		ScheduledFor:    c.ScheduledFor,
		SentAt:          c.SentAt,
		TotalRecipients: int32(c.TotalRecipients),
		SentCount:       int32(c.SentCount),
		FailedCount:     int32(c.FailedCount),
		SkippedCount:    int32(c.SkippedCount),
		OpenedCount:     int32(c.OpenedCount),
		ClickedCount:    int32(c.ClickedCount),
		AbEnabled:       c.ABEnabled,
		AbSubjectB:      c.ABSubjectB,
		AbTestPercent:   int32(c.ABTestPercent),
		AbWinnerMetric:  c.ABWinnerMetric,
		AbTestMinutes:   int32(c.ABTestMinutes),
		AbWinner:        c.ABWinner,
		AbTestStartedAt: c.ABTestStartedAt,
		DeletedAt:       c.DeletedAt,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}
