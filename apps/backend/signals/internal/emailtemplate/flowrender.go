package emailtemplate

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

// Render entry points for the flow engine's transactional send_email node - unlike the built-in RenderOrder* functions, which resolve the slot email and fall back to the hardcoded template, these render a specific flow-chosen library email

func libraryToTemplate(e LibraryEmail) Template {
	cid := e.ChannelID
	return Template{Key: e.Slot, ChannelID: &cid, Subject: e.Subject, Preheader: e.Preheader, Blocks: e.Blocks, Styles: e.Styles}
}

func (s *Service) RenderStoredOrderConfirmation(ctx context.Context, channelID uuid.UUID, e LibraryEmail, d email.OrderConfirmationData) (string, string, string, error) {
	return s.renderTemplate(ctx, channelID, libraryToTemplate(e), s.orderConfirmationCtx(ctx, channelID, d))
}

func (s *Service) RenderStoredOrderShipment(ctx context.Context, channelID uuid.UUID, e LibraryEmail, d email.OrderShipmentData) (string, string, string, error) {
	return s.renderTemplate(ctx, channelID, libraryToTemplate(e), s.orderShipmentCtx(ctx, channelID, d))
}

func (s *Service) RenderStoredOrderCancellation(ctx context.Context, channelID uuid.UUID, e LibraryEmail, d email.OrderCancellationData) (string, string, string, error) {
	return s.renderTemplate(ctx, channelID, libraryToTemplate(e), s.orderCancellationCtx(ctx, channelID, d))
}

func (s *Service) RenderStoredOrderRefund(ctx context.Context, channelID uuid.UUID, e LibraryEmail, d email.OrderRefundData) (string, string, string, error) {
	return s.renderTemplate(ctx, channelID, libraryToTemplate(e), s.orderRefundCtx(ctx, channelID, d))
}
