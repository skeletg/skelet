package webform

import "github.com/google/uuid"

const EventFormSubmitted = "webform.submitted"

// Fires AFTER a submission commits, async - the payload is self-contained so subscribers (admin inbox notification, admin-notify email, submitter confirmation, Pulse flow enrollment) needn't reload the form
type FormSubmitted struct {
	SubmissionID   uuid.UUID         `json:"submissionId"`
	FormID         uuid.UUID         `json:"formId"`
	ChannelID      uuid.UUID         `json:"channelId"`
	FormName       string            `json:"formName"`
	FormSlug       string            `json:"formSlug"`
	Answers        map[string]any    `json:"answers"`
	SubmitterEmail string            `json:"submitterEmail,omitempty"`
	NotifyEmails   []string          `json:"notifyEmails,omitempty"`
	Confirmation   ConfirmationEmail `json:"confirmation"`
	FlowTrigger    bool              `json:"flowTrigger,omitempty"`
	Summary        string            `json:"summary,omitempty"`
}

func (*FormSubmitted) Type() string                { return EventFormSubmitted }
func (e *FormSubmitted) EventChannelID() uuid.UUID { return e.ChannelID }
