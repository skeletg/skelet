package review

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const (
	EventSubmitting = "review.submitting"
	EventSubmitted  = "review.submitted"
	EventApproved   = "review.approved"
	EventRejected   = "review.rejected"
)

// Fires BEFORE a review row is created - sync handlers can abort with a non-nil error or mutate the payload, and the mutable Status lets a spam plugin auto-reject without touching core moderation
type Submitting struct {
	ProductID  uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	GuestEmail string
	Locale     string
	Title      string
	Body       string
	// The proposed initial status - plugins may rewrite it to reject or approve, short-circuiting moderation
	Status *Status
	Tags   *[]string
}

func (*Submitting) Type() string                { return EventSubmitting }
func (e *Submitting) EventChannelID() uuid.UUID { return e.ChannelID }

// Response {reject, reason, status, tags} - status short-circuits moderation and tags append, with the last writer winning on status across webhooks
func (e *Submitting) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		Reject bool     `json:"reject"`
		Reason string   `json:"reason"`
		Status string   `json:"status"`
		Tags   []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if resp.Status != "" {
		status := Status(resp.Status)
		if !status.IsValid() {
			return false, "", fmt.Errorf("webhook response status %q is not a review status", resp.Status)
		}
		if e.Status != nil {
			*e.Status = status
		}
	}
	if len(resp.Tags) > 0 && e.Tags != nil {
		*e.Tags = append(*e.Tags, resp.Tags...)
	}
	return false, "", nil
}

type Submitted struct {
	ReviewID   uuid.UUID
	ProductID  uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	Status     Status
	Tags       []string
}

func (*Submitted) Type() string                { return EventSubmitted }
func (e *Submitted) EventChannelID() uuid.UUID { return e.ChannelID }

// Fires AFTER a moderator approves a pending review - async
type Approved struct {
	ReviewID    uuid.UUID
	ProductID   uuid.UUID
	ChannelID   uuid.UUID
	ModeratorID uuid.UUID
}

func (*Approved) Type() string                { return EventApproved }
func (e *Approved) EventChannelID() uuid.UUID { return e.ChannelID }

type Rejected struct {
	ReviewID    uuid.UUID
	ProductID   uuid.UUID
	ChannelID   uuid.UUID
	ModeratorID uuid.UUID
	Note        string
}

func (*Rejected) Type() string                { return EventRejected }
func (e *Rejected) EventChannelID() uuid.UUID { return e.ChannelID }
