// Package mailsend is the direct transactional send every domain shares - records first, points the open pixel at that record, then delivers
package mailsend

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/email"
	"github.com/skeletg/skelet/apps/backend/internal/emailtrack"
)

// Identifies a send for the log - MessageKey names the template slot it rendered from
type Spec struct {
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	From       string
	To         string
	MessageKey string
	OrderID    *uuid.UUID
}

// Records the send and returns the id the pixel references - structural so a mail path never imports the analytics stack
type Log interface {
	RecordTransactional(ctx context.Context, spec Spec) (uuid.UUID, error)
}

type Sender struct {
	mail  email.Sender
	log   Log
	track *emailtrack.Signer
}

func New(mail email.Sender) *Sender { return &Sender{mail: mail} }

// Without both, mail still goes out - it just doesn't appear in engagement reporting
func (s *Sender) SetTracking(l Log, signer *emailtrack.Signer) { s.log, s.track = l, signer }

// Records before sending, matching the flow engine, so the two agree on what counts as sent - open pixel ONLY, never click-wrapping in front of a reset link
func (s *Sender) Send(ctx context.Context, spec Spec, storefrontURL, subject, htmlBody, textBody string) error {
	if s.log != nil && s.track != nil {
		// A failure here costs reporting, not the mail - the customer still needs what's in it
		if sendID, err := s.log.RecordTransactional(ctx, spec); err != nil {
			log.Printf("[mailsend] record %s to %s: %v", spec.MessageKey, spec.To, err)
		} else {
			htmlBody = s.track.InjectPixel(htmlBody, storefrontURL, sendID)
		}
	}
	return s.mail.Send(ctx, email.Message{
		From: spec.From, To: spec.To, Subject: subject, HTMLBody: htmlBody, TextBody: textBody,
	})
}
