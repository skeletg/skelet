package tracking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
)

type orderReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
	ListCreditMemos(ctx context.Context, orderID uuid.UUID) ([]order.CreditMemo, error)
}

type customerReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (customer.Customer, error)
}

type channelReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error)
}

// Satisfied by *consent.Service
type emailMarketing interface {
	EmailMarketingAllowed(ctx context.Context, channelID uuid.UUID, customerID *uuid.UUID, email string) (bool, error)
}

// Subscribe turns the server's own events into tracking events: purchases, refunds and cancellations off the order
// outbox, cart adds and removals, storefront sign-ups. An order event uses the consent snapshotted at placement, the
// rest the consent the visitor had when they acted
func (s *Service) Subscribe(bus *events.Bus, orders orderReader, channels channelReader, marketing emailMarketing, customers customerReader) {
	if bus == nil {
		return
	}
	h := handlers{svc: s, orders: orders, channels: channels, marketing: marketing, customers: customers}
	bus.Subscribe(order.EventPlaced, h.onPlaced)
	bus.Subscribe(order.EventRefunded, h.onRefunded)
	bus.Subscribe(order.EventCancelled, h.onCancelled)
	bus.Subscribe(cart.EventLineAdded, h.onLineAdded)
	bus.Subscribe(cart.EventBundleAdded, h.onBundleAdded)
	bus.Subscribe(cart.EventLineRemoved, h.onLineRemoved)
	bus.Subscribe(customer.EventRegistered, h.onRegistered)
	bus.Subscribe(consent.EventChanged, h.onConsentChanged)
	bus.Subscribe(customer.EventDeleted, h.onDeleted)
	bus.Subscribe(customer.EventErased, h.onErased)
}

type handlers struct {
	svc       *Service
	orders    orderReader
	channels  channelReader
	marketing emailMarketing // nil: no event says its email may be marketed to
	customers customerReader // nil: an opt-out from the account page reaches no destination
}

// track sends the event, first noting whether its email may receive marketing - a failed lookup goes back to the
// outbox rather than sending the event as if the answer were no
func (h handlers) track(ctx context.Context, ev Event) error {
	if ev.User.Email != "" && h.marketing != nil {
		ok, err := h.marketing.EmailMarketingAllowed(ctx, ev.ChannelID, ev.User.CustomerID, ev.User.Email)
		if err != nil {
			return fmt.Errorf("email marketing consent: %w", err)
		}
		ev.User.MarketingOptIn = ok
	}
	return h.svc.Track(ctx, ev)
}

// A missing order is final; any other failed load goes back to the outbox
func (h handlers) order(ctx context.Context, id uuid.UUID) (order.Order, bool, error) {
	o, err := h.orders.GetByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return order.Order{}, false, nil
	}
	if err != nil {
		return order.Order{}, false, fmt.Errorf("load order %s: %w", id, err)
	}
	return o, true, nil
}

func (h handlers) onPlaced(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderPlaced)
	if !ok {
		return nil
	}
	o, found, err := h.order(ctx, ev.OrderID)
	if !found {
		return err
	}
	return h.track(ctx, PurchaseEvent(o))
}

// PurchaseEvent is the order as GA4's purchase - the id is the order's own, so the browser's copy dedupes against it.
// Its value is the items' net sales: after discount, without tax or shipping, the basis every refund is reported on
func PurchaseEvent(o order.Order) Event {
	ev := orderEvent(o, Purchase, o.ID.String(), o.CreatedAt)
	ev.Ecommerce = &Ecommerce{
		Currency:      o.CurrencyCode,
		TransactionID: o.Number,
		Value:         Decimal(purchaseNet(o), o.CurrencyCode),
		Tax:           Decimal(o.TaxAmount, o.CurrencyCode),
		Shipping:      Decimal(o.ShippingAmount, o.CurrencyCode),
		Items:         orderItems(o, nil),
	}
	return ev
}

func purchaseNet(o order.Order) int64 {
	var net int64
	for _, n := range order.LineNet(o) {
		net += n
	}
	return net
}

// memoNet is what a credit memo takes back of the purchase's value: its lines' net sales, or for a memo naming no
// lines its non-shipping part in the order's own proportion
func memoNet(o order.Order, m order.CreditMemo) int64 {
	var net int64
	if len(m.Lines) > 0 {
		for _, l := range m.Lines {
			net += order.LineNetFor(o, l.OrderLineID, l.Quantity)
		}
	} else if nonShipping := o.TotalAmount - o.ShippingRefundable(); nonShipping > 0 && m.TotalCents > m.ShippingRefundCents {
		net = (m.TotalCents - m.ShippingRefundCents) * purchaseNet(o) / nonShipping
	}
	return min(net, purchaseNet(o))
}

func (h handlers) onRefunded(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderRefunded)
	if !ok {
		return nil
	}
	o, found, err := h.order(ctx, ev.OrderID)
	if !found {
		return err
	}
	// shortcut: a cancelled order's reversal is the cancellation's, so a memo after it (usually returning that same
	// money) reports nothing - a memo for goods shipped before a partial cancel goes unreported; record the cancelled
	// share on the order if that matters
	if o.Status == order.OrderStatusCancelled {
		return nil
	}
	memos, err := h.orders.ListCreditMemos(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("list credit memos of %s: %w", o.ID, err)
	}
	i := slices.IndexFunc(memos, func(m order.CreditMemo) bool { return m.ID == ev.CreditMemoID })
	if i < 0 {
		return nil
	}
	out := RefundEvent(o, memos[i], earlierMemos(o, memos, memos[i]), eventTime(ctx))
	setValueAfter(&out, o, memos, 0)
	return h.track(ctx, out)
}

// setValueAfter is what the order is still worth once every credit memo, and a cancellation's share, is taken off -
// the purchase's own value, tax and shipping in, never below zero
func setValueAfter(ev *Event, o order.Order, memos []order.CreditMemo, cancelled float64) {
	worth := o.SubtotalAmount - o.DiscountAmount + o.TaxAmount + o.ShippingAmount
	left := worth
	for _, m := range memos {
		left -= m.TotalCents
	}
	left -= int64(math.Round(float64(worth) * cancelled))
	v := Decimal(max(left, 0), o.CurrencyCode)
	ev.Ecommerce.ValueAfter = &v
}

// earlierMemos is what the memos before m already took back, so all of them together never exceed the purchase
func earlierMemos(o order.Order, memos []order.CreditMemo, m order.CreditMemo) int64 {
	var sum int64
	for _, x := range memos {
		if x.CreatedAt.Before(m.CreatedAt) || (x.CreatedAt.Equal(m.CreatedAt) && x.ID.String() < m.ID.String()) {
			sum += memoNet(o, x)
		}
	}
	return sum
}

// RefundEvent is one credit memo - partial refunds each report what they gave back, by item where the memo names
// lines, on the purchase's own basis and never past what earlier refunds left of it
func RefundEvent(o order.Order, memo order.CreditMemo, earlier int64, at time.Time) Event {
	qty := map[uuid.UUID]float64{}
	for _, l := range memo.Lines {
		qty[l.OrderLineID] += l.Quantity
	}
	ev := orderEvent(o, Refund, "refund:"+memo.ID.String(), at)
	ev.Ecommerce = &Ecommerce{
		Currency:      o.CurrencyCode,
		TransactionID: o.Number,
		Value:         Decimal(max(min(memoNet(o, memo), purchaseNet(o)-earlier), 0), o.CurrencyCode),
		Shipping:      Decimal(memo.ShippingRefundCents, o.CurrencyCode),
		Items:         orderItems(o, qty),
	}
	return ev
}

func (h handlers) onCancelled(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderCancelled)
	if !ok {
		return nil
	}
	o, found, err := h.order(ctx, ev.OrderID)
	if !found {
		return err
	}
	memos, err := h.orders.ListCreditMemos(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("list credit memos of %s: %w", o.ID, err)
	}
	var refunded int64
	for _, m := range memos {
		refunded += memoNet(o, m)
	}
	out := CancelEvent(o, ev.RefundableFraction, refunded, eventTime(ctx))
	setValueAfter(&out, o, memos, ev.RefundableFraction)
	return h.track(ctx, out)
}

// CancelEvent reverses the purchase a cancelled order reported, scaled to the share that never shipped and less what
// credit memos already refunded - as a refund, which is how the platforms take a reversal
func CancelEvent(o order.Order, fraction float64, refunded int64, at time.Time) Event {
	net := purchaseNet(o)
	ev := orderEvent(o, Refund, "cancel:"+o.ID.String(), at)
	ev.Ecommerce = &Ecommerce{
		Currency:      o.CurrencyCode,
		TransactionID: o.Number,
		Value:         Decimal(max(int64(math.Round(float64(net)*fraction))-refunded, 0), o.CurrencyCode),
	}
	return ev
}

func orderEvent(o order.Order, name, id string, at time.Time) Event {
	ev := Event{
		ID: id, Name: name, OccurredAt: at, ChannelID: o.ChannelID, Source: "server",
		User: User{CustomerID: o.CustomerID, Email: o.Email},
	}
	if o.BillingAddress.Phone != nil {
		ev.User.Phone = *o.BillingAddress.Phone
	}
	ev.WithShopper(o.Tracking)
	return ev
}

// The order's lines as items - all of them, or with qty only those it names, at the quantity given
func orderItems(o order.Order, qty map[uuid.UUID]float64) []Item {
	var items []Item
	for _, l := range o.Lines {
		q := l.Quantity
		if qty != nil {
			if q = qty[l.ID]; q == 0 {
				continue
			}
		}
		items = append(items, Item{ItemID: l.ProductSku, ItemName: l.ProductName, Price: Decimal(l.UnitPrice, o.CurrencyCode), Quantity: q})
	}
	return items
}

func (h handlers) onLineAdded(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineAdded)
	if !ok {
		return nil
	}
	return h.cartEvent(ctx, AddToCart, ev.ChannelID, ev.CustomerID, ev.Shopper, ev.LineID, ev.ProductSku, ev.UnitPrice, ev.Quantity)
}

func (h handlers) onBundleAdded(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.BundleAdded)
	if !ok {
		return nil
	}
	return h.cartEvent(ctx, AddToCart, ev.ChannelID, ev.CustomerID, ev.Shopper, ev.LineID, ev.ProductSku, ev.UnitPrice, ev.Quantity)
}

func (h handlers) onLineRemoved(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineRemoved)
	if !ok || ev.ProductSku == "" {
		return nil
	}
	return h.cartEvent(ctx, RemoveFromCart, ev.ChannelID, ev.CustomerID, ev.Shopper, ev.LineID, ev.ProductSku, ev.UnitPrice, ev.Quantity)
}

// shortcut: cart events ride the bus and can be lost in a crash, move cart.* to the outbox if ad platforms flag gaps
func (h handlers) cartEvent(ctx context.Context, name string, channelID uuid.UUID, cust *uuid.UUID, s *auth.Shopper, lineID uuid.UUID, sku string, unit int64, qty float64) error {
	ch, err := h.channels.GetByID(ctx, channelID)
	if err != nil {
		return fmt.Errorf("load channel %s: %w", channelID, err)
	}
	at := eventTime(ctx)
	ev := Event{
		// The line and the moment, so a replay of this add collapses while a later add to the same line is its own event
		ID: name + ":" + lineID.String() + ":" + strconv.FormatInt(at.UnixMilli(), 10), Name: name,
		OccurredAt: at, ChannelID: channelID, Source: "server", User: User{CustomerID: cust},
		Ecommerce: &Ecommerce{
			Currency: ch.CurrencyCode,
			Value:    Decimal(int64(math.Round(float64(unit)*qty)), ch.CurrencyCode),
			Items:    []Item{{ItemID: sku, Price: Decimal(unit, ch.CurrencyCode), Quantity: qty}},
		},
	}
	ev.WithShopper(s)
	return h.svc.Track(ctx, ev)
}

func (h handlers) onRegistered(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerRegistered)
	if !ok || ev.Source != customer.SourceSelf { // an import, a signup form or an admin isn't a sign-up on the shop
		return nil
	}
	cid := ev.CustomerID
	out := Event{
		ID: "sign_up:" + cid.String(), Name: SignUp, OccurredAt: eventTime(ctx), ChannelID: ev.ChannelID,
		Source: "server", User: User{CustomerID: &cid, Email: ev.Email},
	}
	out.WithShopper(ev.Shopper)
	return h.track(ctx, out)
}

// An account turned email marketing off. Sent as is - the marketing lookup would only confirm the no
func (h handlers) onConsentChanged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*consent.ConsentChanged)
	if !ok || ev.OptedIn || ev.Kind != consentkind.MarketingEmail || h.customers == nil {
		return nil
	}
	c, err := h.customers.GetByID(ctx, ev.CustomerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted since; customer.deleted carries the opt-out
	}
	if err != nil {
		return fmt.Errorf("load customer %s: %w", ev.CustomerID, err)
	}
	phone := ""
	if c.Phone != nil {
		phone = *c.Phone
	}
	return h.optOut(ctx, "optout:", c.ID, c.ChannelID, c.Email, phone)
}

func (h handlers) onDeleted(ctx context.Context, e events.Event) error {
	if ev, ok := e.(*customer.CustomerDeleted); ok {
		return h.optOut(ctx, "deleted:", ev.CustomerID, ev.ChannelID, ev.Email, "")
	}
	return nil
}

func (h handlers) onErased(ctx context.Context, e events.Event) error {
	if ev, ok := e.(*customer.CustomerErased); ok {
		return h.optOut(ctx, "erased:", ev.CustomerID, ev.ChannelID, ev.Email, ev.Phone)
	}
	return nil
}

// The id carries the outbox's emit time, so a redelivery dedupes while a later opt-out of the same person still goes
func (h handlers) optOut(ctx context.Context, prefix string, id, channelID uuid.UUID, email, phone string) error {
	at := eventTime(ctx)
	cid := id
	return h.svc.Track(ctx, Event{
		ID: prefix + id.String() + ":" + strconv.FormatInt(at.UnixMilli(), 10), Name: MarketingOptOut, OccurredAt: at,
		ChannelID: channelID, Source: "server", User: User{CustomerID: &cid, Email: email, Phone: phone},
	})
}

// The outbox's enqueue time on a relayed event, so a redelivery keeps the first attempt's time and id
func eventTime(ctx context.Context) time.Time { return events.EmittedAtOr(ctx, time.Now()).UTC() }
