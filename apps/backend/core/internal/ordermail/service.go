package ordermail

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

// The flow engine, when one is wired - each method reports how many flows took delivery, and a zero hands the send back to this package
// A structural interface so ordermail doesn't import flow, and the snapshot rides along because the engine advances a run on its own tick
type FlowDispatcher interface {
	OnOrderPlaced(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID uuid.UUID, snap email.OrderSnapshot) int
	OnOrderShipped(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID, fulfillmentID uuid.UUID, snap email.OrderSnapshot) int
	OnOrderCancelled(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID uuid.UUID, snap email.OrderSnapshot) int
	OnPaymentRefunded(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID uuid.UUID, ref string, snap email.OrderSnapshot) int
}

// The order reads the templates need - shipment, cancellation and refund events carry neither the
// customer nor the email, so the order is always loaded
type OrderReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
	GetFulfillment(ctx context.Context, id uuid.UUID) (order.Fulfillment, error)
	ListFulfillments(ctx context.Context, orderID uuid.UUID) ([]order.Fulfillment, error)
	PaymentLedgerReader
}

// Split out of OrderReader so the flow engine, which renders the same refund email from a narrower order
// seam of its own, can share RefundedAmount without having to grow to match
type PaymentLedgerReader interface {
	PaymentLedger(ctx context.Context, orderID uuid.UUID) ([]order.PaymentTransaction, error)
}

// Renders a transactional email from the channel's own template, falling back to the built-in body -
// satisfied by wiring.CoreMailRenderer over the templates in internal/email
type Renderer interface {
	RenderOrderConfirmation(ctx context.Context, channelID uuid.UUID, d email.OrderConfirmationData) (string, string, string, error)
	RenderOrderShipment(ctx context.Context, channelID uuid.UUID, d email.OrderShipmentData) (string, string, string, error)
	RenderOrderCancellation(ctx context.Context, channelID uuid.UUID, d email.OrderCancellationData) (string, string, string, error)
	RenderOrderRefund(ctx context.Context, channelID uuid.UUID, d email.OrderRefundData) (string, string, string, error)
}

type ChannelReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error)
}

// Which order emails have already gone out - order id, kind and ref together identify one email exactly
// A structural interface like the seams above, so this package stays off the database
type SendLog interface {
	AlreadySent(ctx context.Context, orderID uuid.UUID, kind, ref string) (bool, error)
	RecordSent(ctx context.Context, orderID uuid.UUID, kind, ref string) error
}

// The kinds recorded against an order, one row per email sent
const (
	kindConfirmation = "confirmation"
	kindShipment     = "shipment"
	kindCancellation = "cancellation"
	kindRefund       = "refund"
)

type Service struct {
	flows    FlowDispatcher // optional - nil sends everything directly
	orders   OrderReader
	channels ChannelReader
	render   Renderer
	sender   *mailsend.Sender
	config   *config.Service
	sendLog  SendLog
}

// sendLog is a constructor argument rather than a setter like the dispatcher below, since a send nothing records goes out again on every replay of its event
func NewService(orders OrderReader, channels ChannelReader, render Renderer, sender *mailsend.Sender, cfg *config.Service, sendLog SendLog) *Service {
	return &Service{orders: orders, channels: channels, render: render, sender: sender, config: cfg, sendLog: sendLog}
}

// Optional - without it every order email goes out directly, which is the correct behaviour for an install with no flow engine
func (s *Service) SetFlowDispatcher(d FlowDispatcher) { s.flows = d }

// A draft, or an order whose contact never resolved - no retry will find an address
var errNoAddress = errors.New("order has no email address")

func (s *Service) send(ctx context.Context, ch channel.Channel, o order.Order, messageKey, subject, htmlBody, textBody string) error {
	if o.Email == "" {
		return fmt.Errorf("%w: %s", errNoAddress, o.Number)
	}
	orderID := o.ID
	if err := s.sender.Send(ctx, mailsend.Spec{
		ChannelID: ch.ID, FallbackName: ch.Name, CustomerID: o.CustomerID, To: o.Email,
		MessageKey: messageKey, OrderID: &orderID,
	}, subject, htmlBody, textBody); err != nil {
		return fmt.Errorf("send %s for order %s: %w", messageKey, o.Number, err)
	}
	return nil
}

// Loads the order and its channel - a failure sends nothing rather than a half-rendered mail, and returns so the event is delivered again, except for an order that no longer exists
func (s *Service) context(ctx context.Context, orderID uuid.UUID) (order.Order, channel.Channel, error) {
	o, err := s.orders.GetByID(ctx, orderID)
	if errors.Is(err, order.ErrOrderNotFound) {
		log.Printf("[ordermail] order %s is gone, no mail", orderID)
		return order.Order{}, channel.Channel{}, errSkip
	}
	if err != nil {
		return order.Order{}, channel.Channel{}, fmt.Errorf("load order %s: %w", orderID, err)
	}
	ch, err := s.channels.GetByID(ctx, o.ChannelID)
	if err != nil {
		return order.Order{}, channel.Channel{}, fmt.Errorf("load channel %s for order %s: %w", o.ChannelID, o.Number, err)
	}
	return o, ch, nil
}

var errSkip = errors.New("skip")

func dropSkip(err error) error {
	if errors.Is(err, errSkip) {
		return nil
	}
	return err
}
