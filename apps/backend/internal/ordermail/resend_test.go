package ordermail

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/email"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

type capturedMail struct{ msgs []email.Message }

func (c *capturedMail) Send(_ context.Context, m email.Message) error {
	c.msgs = append(c.msgs, m)
	return nil
}

type stubRenderer struct{ err error }

func (s stubRenderer) RenderOrderConfirmation(context.Context, uuid.UUID, email.OrderConfirmationData) (string, string, string, error) {
	return "Receipt", "<p>receipt</p>", "receipt", s.err
}

func (s stubRenderer) RenderOrderShipment(context.Context, uuid.UUID, email.OrderShipmentData) (string, string, string, error) {
	return "Shipped", "<p>shipped</p>", "shipped", s.err
}

func (s stubRenderer) RenderOrderCancellation(context.Context, uuid.UUID, email.OrderCancellationData) (string, string, string, error) {
	return "Cancelled", "<p>cancelled</p>", "cancelled", s.err
}

func (s stubRenderer) RenderOrderRefund(context.Context, uuid.UUID, email.OrderRefundData) (string, string, string, error) {
	return "Refunded", "<p>refunded</p>", "refunded", s.err
}

// Counts enrollments so a test can prove the resend path never reached the flow engine
type countingFlows struct{ calls int }

func (c *countingFlows) OnOrderPlaced(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, string) int {
	c.calls++
	return 1
}

func (c *countingFlows) OnOrderShipped(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, uuid.UUID, string) int {
	c.calls++
	return 1
}

func (c *countingFlows) OnOrderCancelled(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, string) int {
	c.calls++
	return 1
}

func (c *countingFlows) OnPaymentRefunded(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, string, string) int {
	c.calls++
	return 1
}

func resendService(o order.Order, fuls []order.Fulfillment, mail *capturedMail) *Service {
	ch := channel.Channel{ID: o.ChannelID, Name: "Shop", PublicURL: "https://shop.example.com"}
	return &Service{
		orders:   fakeOrders{o: o, fulfillments: fuls},
		channels: fakeChannels{ch: ch},
		render:   stubRenderer{},
		sender:   mailsend.New(mail),
	}
}

func placedOrder() order.Order {
	return order.Order{ID: uuid.New(), ChannelID: uuid.New(), Number: "1001", Email: "buyer@example.com"}
}

// The operator asked for this mail now, so handing it back to the flow engine would restart a journey or lose it again
func TestResendNeverConsultsTheFlowEngine(t *testing.T) {
	for _, key := range []string{
		emailtemplate.KeyOrderConfirmation,
		emailtemplate.KeyOrderCancellation,
		emailtemplate.KeyOrderRefund,
	} {
		mail := &capturedMail{}
		svc := resendService(placedOrder(), nil, mail)
		flows := &countingFlows{}
		svc.SetFlowDispatcher(flows)

		if err := svc.Resend(context.Background(), uuid.New(), key, nil); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if flows.calls != 0 {
			t.Errorf("%s: asked the flow engine %d time(s), want the resend to send directly", key, flows.calls)
		}
		if len(mail.msgs) != 1 {
			t.Fatalf("%s: sent %d mails, want exactly 1", key, len(mail.msgs))
		}
		if mail.msgs[0].To != "buyer@example.com" {
			t.Errorf("%s: sent to %q, want the order's address", key, mail.msgs[0].To)
		}
	}
}

// An operator picking "resend the shipping email" on a multi-shipment order means the latest one
func TestResendShipmentDefaultsToTheNewestFulfillment(t *testing.T) {
	o := placedOrder()
	now := time.Now()
	newest := uuid.New()
	fuls := []order.Fulfillment{
		{ID: uuid.New(), OrderID: o.ID, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: newest, OrderID: o.ID, CreatedAt: now},
		{ID: uuid.New(), OrderID: o.ID, CreatedAt: now.Add(-1 * time.Hour)},
	}
	svc := resendService(o, fuls, &capturedMail{})
	got, err := svc.resendTarget(context.Background(), o, nil)
	if err != nil {
		t.Fatalf("resendTarget: %v", err)
	}
	if got != newest {
		t.Errorf("picked %v, want the newest fulfillment %v", got, newest)
	}
}

// The resolver gates on the ORDER's channel, so honouring a foreign fulfillment would mail from a channel the admin was never granted
func TestResendShipmentRejectsAFulfillmentFromAnotherOrder(t *testing.T) {
	o := placedOrder()
	fuls := []order.Fulfillment{{ID: uuid.New(), OrderID: o.ID, CreatedAt: time.Now()}}
	svc := resendService(o, fuls, &capturedMail{})

	foreign := uuid.New()
	if _, err := svc.resendTarget(context.Background(), o, &foreign); err == nil {
		t.Fatal("accepted a fulfillment from another order, want it refused")
	}

	mail := &capturedMail{}
	svc = resendService(o, fuls, mail)
	if err := svc.Resend(context.Background(), o.ID, emailtemplate.KeyOrderShipment, &foreign); err == nil {
		t.Error("Resend accepted a foreign fulfillment id")
	}
	if len(mail.msgs) != 0 {
		t.Errorf("sent %d mails on a refused resend, want none", len(mail.msgs))
	}
}

func TestResendShipmentNeedsAShipment(t *testing.T) {
	o := placedOrder()
	mail := &capturedMail{}
	svc := resendService(o, nil, mail)
	err := svc.Resend(context.Background(), o.ID, emailtemplate.KeyOrderShipment, nil)
	if err == nil {
		t.Fatal("resent a shipment notice for an order with no shipments")
	}
	if !strings.Contains(err.Error(), "no shipment") {
		t.Errorf("error = %q, want it to name the missing shipment", err)
	}
	if len(mail.msgs) != 0 {
		t.Errorf("sent %d mails, want none", len(mail.msgs))
	}
}

// Only the four lifecycle emails - anything else would render from a template slot this path can't fill
func TestResendRejectsAnUnknownMessage(t *testing.T) {
	mail := &capturedMail{}
	svc := resendService(placedOrder(), nil, mail)
	if err := svc.Resend(context.Background(), uuid.New(), "password_reset", nil); err == nil {
		t.Fatal("accepted a non-order message key")
	}
	if len(mail.msgs) != 0 {
		t.Errorf("sent %d mails for an unknown key, want none", len(mail.msgs))
	}
}

// A resend is a foreground action, so the operator hears about a failure instead of it going to the log
func TestResendReportsFailures(t *testing.T) {
	o := placedOrder()
	svc := resendService(o, nil, &capturedMail{})
	svc.render = stubRenderer{err: errors.New("template broken")}
	if err := svc.Resend(context.Background(), o.ID, emailtemplate.KeyOrderConfirmation, nil); err == nil {
		t.Error("render failure was swallowed, want it returned")
	}

	svc = resendService(order.Order{ID: o.ID, ChannelID: o.ChannelID, Number: "1002"}, nil, &capturedMail{})
	if err := svc.Resend(context.Background(), o.ID, emailtemplate.KeyOrderConfirmation, nil); err == nil {
		t.Error("resent to an order with no email address, want an error")
	}
}
