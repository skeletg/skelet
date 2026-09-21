package wiring

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

// Core renders its own transactional mail from the templates compiled into internal/email
// The channel id the seams pass is unused: these templates take their store name, logo and links in the render data

type CoreMailRenderer struct{}

func (CoreMailRenderer) RenderOrderConfirmation(_ context.Context, _ uuid.UUID, d email.OrderConfirmationData) (string, string, string, error) {
	return email.RenderOrderConfirmation(d)
}

func (CoreMailRenderer) RenderOrderShipment(_ context.Context, _ uuid.UUID, d email.OrderShipmentData) (string, string, string, error) {
	return email.RenderOrderShipment(d)
}

func (CoreMailRenderer) RenderOrderCancellation(_ context.Context, _ uuid.UUID, d email.OrderCancellationData) (string, string, string, error) {
	return email.RenderOrderCancellation(d)
}

func (CoreMailRenderer) RenderOrderRefund(_ context.Context, _ uuid.UUID, d email.OrderRefundData) (string, string, string, error) {
	return email.RenderOrderRefund(d)
}

// Account mail renders the same way and sends itself, since nothing downstream of the render differs
type CoreAccountMail struct{ sender *mailsend.Sender }

func NewCoreAccountMail(s *mailsend.Sender) CoreAccountMail { return CoreAccountMail{sender: s} }

func (c CoreAccountMail) SendVerification(ctx context.Context, channelID uuid.UUID, from, to, _ string, d email.VerificationEmailData) error {
	subject, html, text, err := email.RenderVerificationEmail(d)
	if err != nil {
		return err
	}
	return c.send(ctx, channelID, from, to, email.KeyVerification, subject, html, text)
}

func (c CoreAccountMail) SendPasswordReset(ctx context.Context, channelID uuid.UUID, from, to, _ string, d email.PasswordResetEmailData) error {
	subject, html, text, err := email.RenderPasswordResetEmail(d)
	if err != nil {
		return err
	}
	return c.send(ctx, channelID, from, to, email.KeyPasswordReset, subject, html, text)
}

func (c CoreAccountMail) send(ctx context.Context, channelID uuid.UUID, from, to, key, subject, html, text string) error {
	return c.sender.Send(ctx, mailsend.Spec{
		ChannelID: channelID, From: from, To: to, MessageKey: key,
	}, subject, html, text)
}
