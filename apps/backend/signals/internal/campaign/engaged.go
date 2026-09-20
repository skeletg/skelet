package campaign

import "github.com/google/uuid"

// The two engagements a send reports, machine hits excluded
const (
	KindOpened  = "email_opened"
	KindClicked = "email_clicked"
)

// Engaged is a human open or click by a known customer - what the tracking routes hand the flow
// engine to start an engagement-triggered flow on
type Engaged struct {
	Kind       string // KindOpened | KindClicked
	ChannelID  uuid.UUID
	CustomerID uuid.UUID
	CampaignID *uuid.UUID
	FlowID     *uuid.UUID
}
