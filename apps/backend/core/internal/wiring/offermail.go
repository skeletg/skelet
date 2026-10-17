package wiring

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	graphadmin "github.com/skeletg/skelet/apps/backend/core/graph/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/core/internal/ordermail"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient/enroll"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// OfferMail sends an offer cart's link like the account mail goes out - offered to a Pulse flow first, and from core's own template when no flow takes it, so the admin's send always reaches the recipient
type OfferMail struct {
	flows     *enroll.Enroller
	sender    *mailsend.Sender
	cfg       *config.Service
	customers *customer.Service
}

func NewOfferMail(flows *enroll.Enroller, sender *mailsend.Sender, cfg *config.Service, customers *customer.Service) OfferMail {
	return OfferMail{flows: flows, sender: sender, cfg: cfg, customers: customers}
}

// offerMessage is what the mail says - one reading for the send and the console's preview, so the preview is the mail
type offerMessage struct {
	data       email.CartOfferEmailData
	customerID uuid.UUID
}

func (o OfferMail) compose(ctx context.Context, m graphadmin.OfferMail) offerMessage {
	c := m.Cart
	out := offerMessage{data: email.CartOfferEmailData{
		StoreName: ordermail.StoreName(ctx, o.cfg, *m.Channel),
		Link:      m.Link,
		Message:   c.Offer.Message,
		ExpiresAt: c.Offer.ExpiresAt.Format("2 January 2006, 15:04 MST"),
	}}
	if c.CustomerID != nil {
		out.customerID = *c.CustomerID
		if o.customers != nil {
			if cu, err := o.customers.GetByID(ctx, out.customerID); err == nil {
				out.data.FirstName = cu.FirstName
			}
		}
	}
	var subtotal int64
	for _, l := range c.Lines {
		lt := money.LineTotal(l.UnitPrice, l.Quantity)
		subtotal += lt
		out.data.Lines = append(out.data.Lines, email.CartOfferLine{Name: l.ProductName, Quantity: strconv.FormatFloat(l.Quantity, 'f', -1, 64), Total: email.FormatMoney(lt, m.Currency)})
	}
	out.data.Total = email.FormatMoney(subtotal, m.Currency)
	return out
}

func (o OfferMail) SendCartOffer(ctx context.Context, m graphadmin.OfferMail) error {
	c := m.Cart
	msg := o.compose(ctx, m)
	d := msg.data
	if o.flows != nil && o.flows.OnCartOffer(ctx, c.ChannelID, msg.customerID, m.To, c.ID, m.Link, d.ExpiresAt, d.Message, d.StoreName, d.FirstName) > 0 {
		return nil
	}
	subject, html, text, err := email.RenderCartOfferEmail(d)
	if err != nil {
		return err
	}
	spec := mailsend.Spec{ChannelID: c.ChannelID, To: m.To, MessageKey: email.KeyCartOffer, FallbackName: d.StoreName}
	if c.CustomerID != nil {
		spec.CustomerID = c.CustomerID
	}
	return o.sender.Send(ctx, spec, subject, html, text)
}

// PreviewCartOffer renders the mail core sends, without sending it - a Pulse flow on cart_offer, when one is active, sends its own design instead
func (o OfferMail) PreviewCartOffer(ctx context.Context, m graphadmin.OfferMail) (graphadmin.OfferMailPreview, error) {
	subject, html, text, err := email.RenderCartOfferEmail(o.compose(ctx, m).data)
	if err != nil {
		return graphadmin.OfferMailPreview{}, err
	}
	return graphadmin.OfferMailPreview{Subject: subject, HTML: html, Text: text}, nil
}
