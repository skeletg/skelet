package sendlog

import "github.com/google/uuid"

// The two engagements a send reports, machine hits excluded
const (
	KindOpened  = "email_opened"
	KindClicked = "email_clicked"
)

// Bus topics the flow engine subscribes to, so an open or click can start an engagement-triggered flow
const (
	BusOpened  = "analytics." + KindOpened
	BusClicked = "analytics." + KindClicked
)

// Engaged fires for a human open or click by a known customer - satisfies events.Event structurally, so this package stays a leaf
type Engaged struct {
	Kind       string // KindOpened | KindClicked
	ChannelID  uuid.UUID
	CustomerID uuid.UUID
	CampaignID *uuid.UUID
	FlowID     *uuid.UUID
}

func (e *Engaged) Type() string { return "analytics." + e.Kind }
