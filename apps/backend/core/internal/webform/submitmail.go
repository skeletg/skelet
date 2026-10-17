package webform

import (
	"context"
	"html"
	"log/slog"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/mailsend"
)

// Named so submission mail is distinguishable in the send record
const (
	MessageKeyFormNotify  = "form_submission_notify"
	MessageKeyFormConfirm = "form_submission_confirm"
)

// RegisterSubmitEmails notifies the form's recipients and, when the form opts in, confirms to the submitter. The admin
// inbox notification is a separate subscriber, and a Pulse flow trigger is a planned follow-up
func RegisterSubmitEmails(bus *events.Bus, sender *mailsend.Sender) {
	bus.Subscribe(EventFormSubmitted, func(ctx context.Context, e events.Event) error { // On submit: email the admin recipients + (optionally) confirm to the submitter - admin inbox notification is handled by the notification subscriber; the Pulse flow trigger is a planned follow-up
		ev, ok := e.(*FormSubmitted)
		if !ok {
			return nil
		}
		for _, to := range ev.NotifyEmails {
			if err := sender.Send(ctx, mailsend.Spec{ChannelID: ev.ChannelID, To: to, MessageKey: MessageKeyFormNotify},
				"New submission: "+ev.FormName,
				"<pre style=\"font-family:sans-serif;white-space:pre-wrap\">"+html.EscapeString(ev.Summary)+"</pre>",
				ev.Summary); err != nil {
				slog.ErrorContext(ctx, "webform: submission notice not sent", "form", ev.FormID, "err", err)
			}
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
			if err := sender.Send(ctx, mailsend.Spec{ChannelID: ev.ChannelID, To: ev.SubmitterEmail, MessageKey: MessageKeyFormConfirm},
				subject,
				"<p style=\"font-family:sans-serif\">"+html.EscapeString(body)+"</p>",
				body); err != nil {
				slog.ErrorContext(ctx, "webform: submission confirmation not sent", "form", ev.FormID, "err", err)
			}
		}
		return nil
	})
}
