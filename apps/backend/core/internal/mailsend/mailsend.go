// Package mailsend is the one path core sends mail on - it resolves who the mail is from, delivers it, and tells Signals it happened
package mailsend

import (
	"context"
	"log"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

// Identifies a send for reporting - MessageKey names the template slot it rendered from
type Spec struct {
	// uuid.Nil addresses the platform rather than a shop, which is what staff mail wants
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	To         string
	MessageKey string
	OrderID    *uuid.UUID
	// Set only where the caller already holds a resolved identity - account mail reaches here through a projection that has one
	From string
	// What to call the sender when the channel never configured a name
	FallbackName string
}

// Tells the analytics spine a mail went out - structural so a mail path never imports the suite's client
type Recorder interface {
	RecordSent(ctx context.Context, sendID uuid.UUID, spec Spec) error
}

type Sender struct {
	mail email.Sender
	cfg  *config.Service
	rec  Recorder
}

func New(mail email.Sender, cfg *config.Service) *Sender { return &Sender{mail: mail, cfg: cfg} }

// Without one the mail still goes out, it just never reaches engagement reporting
func (s *Sender) SetRecorder(r Recorder) { s.rec = r }

// Delivers first and reports after, so a send that never left doesn't count as one
func (s *Sender) Send(ctx context.Context, spec Spec, subject, htmlBody, textBody string) error {
	if err := s.mail.Send(ctx, email.Message{
		From: s.from(ctx, spec), To: spec.To, Subject: subject, HTMLBody: htmlBody, TextBody: textBody,
	}); err != nil {
		return err
	}
	if s.rec != nil {
		if err := s.rec.RecordSent(ctx, uuid.New(), spec); err != nil {
			log.Printf("[mailsend] record %s to %s: %v", spec.MessageKey, spec.To, err)
		}
	}
	return nil
}

// The channel's configured sender identity, or the platform's when the mail belongs to no shop
func (s *Sender) from(ctx context.Context, spec Spec) string {
	if spec.From != "" {
		return spec.From
	}
	scope := config.DefaultScope
	if spec.ChannelID != uuid.Nil {
		id := spec.ChannelID
		scope = config.Scope{Kind: config.ScopeChannel, ChannelID: &id}
	}
	addr, name := "no-reply@skelet.local", ""
	if s.cfg != nil {
		if a, _ := s.cfg.GetString(ctx, "general/contact/email_from_address", scope); strings.TrimSpace(a) != "" {
			addr = strings.TrimSpace(a)
		}
		n, _ := s.cfg.GetString(ctx, "general/contact/email_from_name", scope)
		name = strings.TrimSpace(n)
	}
	if name == "" {
		name = spec.FallbackName
	}
	if name == "" {
		return addr
	}
	return name + " <" + addr + ">"
}
