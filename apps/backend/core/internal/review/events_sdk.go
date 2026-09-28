package review

import (
	"fmt"

	"github.com/google/uuid"
)

// Accessors for sdk.ReviewSubmitting, pinned in internal/plugin - the status and tags follow the webhook's rules, and
// both are no-ops when the emitter left their pointers nil, exactly as the webhook's are

func (e *Submitting) ReviewProductID() uuid.UUID   { return e.ProductID }
func (e *Submitting) ReviewChannelID() uuid.UUID   { return e.ChannelID }
func (e *Submitting) ReviewCustomerID() *uuid.UUID { return e.CustomerID }
func (e *Submitting) ReviewGuestEmail() string     { return e.GuestEmail }
func (e *Submitting) ReviewLocale() string         { return e.Locale }
func (e *Submitting) ReviewTitle() string          { return e.Title }
func (e *Submitting) ReviewBody() string           { return e.Body }

func (e *Submitting) ReviewStatus() string {
	if e.Status == nil {
		return ""
	}
	return string(*e.Status)
}

func (e *Submitting) ReviewTags() []string {
	if e.Tags == nil {
		return nil
	}
	return append([]string(nil), (*e.Tags)...)
}

func (e *Submitting) SetReviewStatus(status string) error {
	s := Status(status)
	if !s.IsValid() {
		return fmt.Errorf("%q is not a review status", status)
	}
	if e.Status != nil {
		*e.Status = s
	}
	return nil
}

func (e *Submitting) AddReviewTags(tags ...string) {
	if e.Tags != nil {
		*e.Tags = append(*e.Tags, tags...)
	}
}
