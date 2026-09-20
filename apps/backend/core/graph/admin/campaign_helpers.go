package admin

import (
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func campaignInput(in model.SaveCampaignInput) (*signalsv1.CampaignInput, error) {
	blocks, err := jsonText(in.Blocks)
	if err != nil {
		return nil, errInvalidInput("blocks", err)
	}
	styles, err := jsonText(in.Styles)
	if err != nil {
		return nil, errInvalidInput("styles", err)
	}
	return &signalsv1.CampaignInput{
		ChannelId: in.ChannelID, Name: in.Name, EmailId: in.EmailID, Subject: in.Subject, BlocksJson: blocks, StylesJson: styles,
		AudienceKind: in.AudienceKind, AudienceId: in.AudienceID,
		AbEnabled:      in.AbEnabled != nil && *in.AbEnabled,
		AbSubjectB:     optionalStr(in.AbSubjectB),
		AbTestPercent:  optionalInt(in.AbTestPercent),
		AbWinnerMetric: optionalStr(in.AbWinnerMetric),
		AbTestMinutes:  optionalInt(in.AbTestMinutes),
	}, nil
}

func campaignModel(c *signalsv1.Campaign) *model.EmailCampaign {
	return &model.EmailCampaign{
		ID:              c.GetId(),
		ChannelID:       c.GetChannelId(),
		Name:            c.GetName(),
		EmailID:         c.EmailId,
		Subject:         c.GetSubject(),
		Blocks:          jsonValue(c.GetBlocksJson(), "[]"),
		Styles:          jsonValue(c.GetStylesJson(), "{}"),
		AudienceKind:    c.GetAudienceKind(),
		AudienceID:      c.AudienceId,
		Status:          c.GetStatus(),
		ScheduledFor:    protoTimePtr(c.ScheduledFor),
		SentAt:          protoTimePtr(c.SentAt),
		TotalRecipients: c.GetTotalRecipients(),
		SentCount:       c.GetSentCount(),
		FailedCount:     c.GetFailedCount(),
		SkippedCount:    c.GetSkippedCount(),
		OpenedCount:     c.GetOpenedCount(),
		ClickedCount:    c.GetClickedCount(),
		AbEnabled:       c.GetAbEnabled(),
		AbSubjectB:      c.GetAbSubjectB(),
		AbTestPercent:   c.GetAbTestPercent(),
		AbWinnerMetric:  c.GetAbWinnerMetric(),
		AbTestMinutes:   c.GetAbTestMinutes(),
		AbWinner:        c.GetAbWinner(),
		AbTestStartedAt: protoTimePtr(c.AbTestStartedAt),
		DeletedAt:       protoTimePtr(c.DeletedAt),
		CreatedAt:       protoTime(c.GetCreatedAt()),
		UpdatedAt:       protoTime(c.GetUpdatedAt()),
	}
}
