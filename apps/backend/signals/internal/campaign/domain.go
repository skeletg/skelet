// Package campaign implements email campaigns and broadcasts - an admin composes an email using the same typed-block model as transactional templates and sends it to an audience, now or scheduled, and a scheduler code job claims due campaigns, materializes the recipient list filtered by marketing-email consent, and sends per recipient
package campaign

import (
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

const ( // campaign lifecycle - validated in Go, with no DB enum
	StatusDraft     = "draft"
	StatusScheduled = "scheduled"
	StatusSending   = "sending"
	StatusTesting   = "testing" // A/B: test slice sent, measurement window running
	StatusSent      = "sent"
	StatusCancelled = "cancelled"
)

const (
	ABMetricOpen  = "open"
	ABMetricClick = "click"
	VariantA      = "a"
	VariantB      = "b"
	PhaseSend     = "send"
	PhaseTest     = "test"
	PhaseHoldout  = "holdout"
)

const (
	AudienceAll     = "all"
	AudienceGroup   = "group"
	AudienceSegment = "segment"
)

type Campaign struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Name      string
	// References a reusable library email and supplies the content when set - Subject, Blocks and Styles are the inline fallback for a nil EmailID
	EmailID         *uuid.UUID
	Subject         string
	Blocks          []emailtemplate.Block
	Styles          emailtemplate.EmailStyles
	AudienceKind    string
	AudienceID      *uuid.UUID
	Status          string
	ScheduledFor    *time.Time
	SentAt          *time.Time
	TotalRecipients int
	SentCount       int
	FailedCount     int
	SkippedCount    int
	OpenedCount     int
	ClickedCount    int
	// A/B subject test, inline-content campaigns only - with it on, a TestPercent slice of the audience splits between Subject and ABSubjectB, and the higher WinnerMetric rate's subject goes to the rest
	ABEnabled       bool
	ABSubjectB      string
	ABTestPercent   int
	ABWinnerMetric  string
	ABTestMinutes   int
	ABWinner        string // '' until decided, then 'a' | 'b'
	ABTestStartedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type Recipient struct { // one targeted address, and also the at-least-once send-log row
	ID         uuid.UUID
	CampaignID uuid.UUID
	CustomerID *uuid.UUID
	Email      string
	FirstName  string
	Status     string
	Error      string
	Variant    string
	Phase      string
}

type VariantStat struct { // one A/B variant's tally, feeding the results view and the winner decision
	Variant string
	Sent    int
	Opened  int
	Clicked int
}

func validAudience(kind string) bool {
	switch kind {
	case AudienceAll, AudienceGroup, AudienceSegment:
		return true
	}
	return false
}
