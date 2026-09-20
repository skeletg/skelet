package signalsclient

import (
	"context"

	"github.com/google/uuid"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The two send-log reads core's own work makes - a flow condition splitting on a customer's engagement,
// and the A/B winner a campaign picks - both as core itself, since a scheduled run has no admin

// EmailEngagementCounts is a customer's lifetime email engagement, machine opens excluded
type EmailEngagementCounts struct {
	Sent    int
	Opened  int
	Clicked int
}

func (c *Client) CustomerEmailEngagement(ctx context.Context, customerID uuid.UUID) (EmailEngagementCounts, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return EmailEngagementCounts{}, err
	}
	res, err := c.Pulse.GetCustomerEngagement(sys, &signalsv1.GetCustomerEngagementRequest{CustomerId: customerID.String()})
	if err != nil {
		return EmailEngagementCounts{}, err
	}
	return EmailEngagementCounts{Sent: int(res.GetSent()), Opened: int(res.GetOpened()), Clicked: int(res.GetClicked())}, nil
}

// VariantStat is one A/B arm's sent, opened and clicked, human hits only
type VariantStat struct {
	Variant string
	Sent    int
	Opened  int
	Clicked int
}

func (c *Client) CampaignVariantStats(ctx context.Context, campaignID uuid.UUID) ([]VariantStat, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return nil, err
	}
	res, err := c.Pulse.GetCampaignVariantStats(sys, &signalsv1.GetCampaignVariantStatsRequest{CampaignId: campaignID.String()})
	if err != nil {
		return nil, err
	}
	out := make([]VariantStat, 0, len(res.GetVariants()))
	for _, v := range res.GetVariants() {
		out = append(out, VariantStat{Variant: v.GetVariant(), Sent: int(v.GetSent()), Opened: int(v.GetOpened()), Clicked: int(v.GetClicked())})
	}
	return out, nil
}
