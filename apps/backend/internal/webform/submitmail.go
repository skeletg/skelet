package webform

import (
	"context"
	"html"

	"github.com/skeletg/skelet/internal/email"
	"github.com/skeletg/skelet/internal/events"
)

// Default sender for submission mail - these are operational notifications rather
// than marketing, so they don't route through a channel's configured from-address
const submitMailFrom = "no-reply@skelet.local"

// RegisterSubmitEmails notifies the form's recipients and, when the form opts in, confirms to the submitter. The admin
// inbox notification is a separate subscriber, and a Pulse flow trigger is a planned follow-up
func RegisterSubmitEmails(bus *events.Bus, sender email.Sender) {
	bus.Subscribe(EventFormSubmitted, func(ctx context.Context, e events.Event) error { // On submit: email the admin recipients + (optionally) confirm to the submitter - admin inbox notification is handled by the notification subscriber; the Pulse flow trigger is a planned follow-up
		ev, ok := e.(*FormSubmitted)
		if !ok {
			return nil
		}
		for _, to := range ev.NotifyEmails {
			_ = sender.Send(ctx, email.Message{
				From:     submitMailFrom,
				To:       to,
				Subject:  "New submission: " + ev.FormName,
				TextBody: ev.Summary,
				HTMLBody: "<pre style=\"font-family:sans-serif;white-space:pre-wrap\">" + html.EscapeString(ev.Summary) + "</pre>",
			})
		}
		if ev.Confirmation.Enabled && ev.SubmitterEmail != "" {
			subject := ev.Confirmation.Subject
			if subject == "" {
				subject = "We received your submission"
			}
			body := ev.Confirmation.Body
			if body == "" {
				body = "Thanks - we've received your submission and will be in touch."
			}
			_ = sender.Send(ctx, email.Message{
				From:     submitMailFrom,
				To:       ev.SubmitterEmail,
				Subject:  subject,
				TextBody: body,
				HTMLBody: "<p style=\"font-family:sans-serif\">" + html.EscapeString(body) + "</p>",
			})
		}
		return nil
	})
}
