package ordermail

import (
	"context"
	"log"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/email"
	"github.com/skeletg/skelet/apps/backend/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// The flow engine, when one is wired - each method reports how many flows took delivery, and a zero
// is what hands the send back to this package
// A structural interface so ordermail doesn't import flow, which imports this package for the mapping above
type FlowDispatcher interface {
	OnOrderPlaced(ctx context.Context, channelID, customerID uuid.UUID, email string, orderID uuid.UUID, orderNumber string) int
	OnOrderShipped(ctx context.Context, channelID, customerID uuid.UUID, email string, orderID, fulfillmentID uuid.UUID, orderNumber string) int
	OnOrderCancelled(ctx context.Context, channelID, customerID uuid.UUID, email string, orderID uuid.UUID, orderNumber string) int
	OnPaymentRefunded(ctx context.Context, channelID, customerID uuid.UUID, email string, orderID uuid.UUID, orderNumber, ref string) int
}

// The order reads the templates need - shipment, cancellation and refund events carry neither the
// customer nor the email, so the order is always loaded
type OrderReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
	GetFulfillment(ctx context.Context, id uuid.UUID) (order.Fulfillment, error)
	PaymentLedger(ctx context.Context, orderID uuid.UUID) ([]order.PaymentTransaction, error)
}

// Renders a transactional email from the channel's own template, falling back to the built-in body -
// satisfied by *emailtemplate.Service
type Renderer interface {
	RenderOrderConfirmation(ctx context.Context, channelID uuid.UUID, d email.OrderConfirmationData) (string, string, string, error)
	RenderOrderShipment(ctx context.Context, channelID uuid.UUID, d email.OrderShipmentData) (string, string, string, error)
	RenderOrderCancellation(ctx context.Context, channelID uuid.UUID, d email.OrderCancellationData) (string, string, string, error)
	RenderOrderRefund(ctx context.Context, channelID uuid.UUID, d email.OrderRefundData) (string, string, string, error)
}

type ChannelReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error)
}

type Service struct {
	flows    FlowDispatcher // optional - nil sends everything directly
	orders   OrderReader
	channels ChannelReader
	render   Renderer
	sender   *mailsend.Sender
	config   *config.Service
}

func NewService(orders OrderReader, channels ChannelReader, render Renderer, sender *mailsend.Sender, cfg *config.Service) *Service {
	return &Service{orders: orders, channels: channels, render: render, sender: sender, config: cfg}
}

// Optional - without it every order email goes out directly, which is the correct behaviour for an install with no flow engine
func (s *Service) SetFlowDispatcher(d FlowDispatcher) { s.flows = d }

// The From header, matching what the flow engine builds so a direct send is indistinguishable from a flow one
func (s *Service) fromAddress(ctx context.Context, ch channel.Channel) string {
	addr, name := "no-reply@skelet.local", ""
	if s.config != nil {
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}
		if a, _ := s.config.GetString(ctx, "general/contact/email_from_address", scope); strings.TrimSpace(a) != "" {
			addr = strings.TrimSpace(a)
		}
		name, _ = s.config.GetString(ctx, "general/contact/email_from_name", scope)
		name = strings.TrimSpace(name)
	}
	if name == "" {
		name = ch.Name
	}
	if name != "" {
		return name + " <" + addr + ">"
	}
	return addr
}

func (s *Service) send(ctx context.Context, ch channel.Channel, o order.Order, messageKey, subject, htmlBody, textBody string) {
	if o.Email == "" {
		return // nothing to send to - a draft or an order whose contact never resolved
	}
	orderID := o.ID
	if err := s.sender.Send(ctx, mailsend.Spec{
		ChannelID: ch.ID, CustomerID: o.CustomerID, From: s.fromAddress(ctx, ch), To: o.Email,
		MessageKey: messageKey, OrderID: &orderID,
	}, StorefrontURL(ch), subject, htmlBody, textBody); err != nil {
		log.Printf("[ordermail] %s to %s for order %s: %v", messageKey, o.Email, o.Number, err)
	}
}

// Loads the order and its channel - either failing skips the mail rather than sending a half-rendered one
func (s *Service) context(ctx context.Context, orderID uuid.UUID) (order.Order, channel.Channel, bool) {
	o, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[ordermail] load order %s: %v", orderID, err)
		return order.Order{}, channel.Channel{}, false
	}
	ch, err := s.channels.GetByID(ctx, o.ChannelID)
	if err != nil {
		log.Printf("[ordermail] load channel %s for order %s: %v", o.ChannelID, o.Number, err)
		return order.Order{}, channel.Channel{}, false
	}
	return o, ch, true
}
