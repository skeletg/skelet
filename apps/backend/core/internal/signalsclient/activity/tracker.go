// Package activity subscribes to the domain event bus and turns every customer-facing action (commerce + account) into an append to the analytics spine, so the profile timeline reflects a customer's full journey
// not just web behaviour and email
package activity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Satisfied by *signalsclient.Client - an interface so the tracker tests without a service
type activityRecorder interface {
	RecordActivity(ctx context.Context, a signalsclient.ActivitySpec) error
}

// The spine's source column for what this package emits - core writes only these two
const (
	sourceCommerce = "commerce"
	sourceAccount  = "account"
)

const signalsWriteTimeout = 2 * time.Second

type orderLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
}

type Tracker struct {
	rec    activityRecorder
	orders orderLookup
}

func NewTracker(rec *signalsclient.Client, orders *order.Service) *Tracker {
	return &Tracker{rec: rec, orders: orders}
}

func (t *Tracker) Subscribe(bus *events.Bus) {
	if bus == nil {
		return
	}
	// commerce
	bus.Subscribe(order.EventPlaced, t.onOrderPlaced)
	bus.Subscribe(order.EventStatusChanged, t.onOrderStatusChanged)
	bus.Subscribe(order.EventPaymentStatusChanged, t.onOrderPayment)
	bus.Subscribe(order.EventFulfillmentCreated, t.onOrderShipped)
	bus.Subscribe(order.EventReturnReceived, t.onReturnReceived)
	bus.Subscribe(order.EventReturnRequested, t.onReturnRequested)
	bus.Subscribe(order.EventReturnApproved, t.onReturnApproved)
	bus.Subscribe(order.EventReturnRejected, t.onReturnRejected)
	bus.Subscribe(order.EventEdited, t.onOrderEdited)
	bus.Subscribe(cart.EventLineAdded, t.onCartAdd)
	bus.Subscribe(cart.EventBundleAdded, t.onCartBundleAdd)
	bus.Subscribe(cart.EventLineUpdated, t.onCartUpdate)
	bus.Subscribe(cart.EventLineRemoved, t.onCartRemove)
	bus.Subscribe(cart.EventCleared, t.onCartCleared)
	bus.Subscribe(cart.EventMerged, t.onCartMerged)
	bus.Subscribe(review.EventSubmitted, t.onReviewSubmitted)
	bus.Subscribe(subscription.EventCreated, t.onSubLifecycle)
	bus.Subscribe(subscription.EventCanceled, t.onSubLifecycle)
	bus.Subscribe(subscription.EventPaused, t.onSubLifecycle)
	bus.Subscribe(subscription.EventResumed, t.onSubLifecycle)
	bus.Subscribe(subscription.EventInvoiced, t.onSubInvoiced)
	bus.Subscribe(download.EventAccessed, t.onDownloadAccessed)
	// account
	bus.Subscribe(customer.EventRegistered, t.onRegistered)
	bus.Subscribe(customer.EventEmailVerified, t.onEmailVerified)
	bus.Subscribe(customer.EventUpdated, t.onAccountUpdated)
	bus.Subscribe(customer.EventProfileUpdated, t.onProfileUpdated)
	bus.Subscribe(customer.EventPasswordChanged, t.onPasswordChanged)
	bus.Subscribe(customer.EventAddressChanged, t.onAddressChanged)
	bus.Subscribe(auth.EventLoginSucceeded, t.onLogin)
	bus.Subscribe(auth.EventLogout, t.onLogout)
	bus.Subscribe(auth.EventPasswordReset, t.onPasswordReset)
}

func (t *Tracker) emit(ctx context.Context, source, name string, channel uuid.UUID, cust *uuid.UUID, dedupe string, props map[string]any) error {
	return t.emitID(ctx, source, name, channel, cust, "", dedupe, props)
}

// emitID also accepts an anon id for guest commerce - No-op when neither a
// customer nor an anon id is present
// An unreachable suite hands the event back to the outbox, whose replay the dedupe key collapses - a refusal is final
func (t *Tracker) emitID(ctx context.Context, source, name string, channel uuid.UUID, cust *uuid.UUID, anon, dedupe string, props map[string]any) error {
	if cust == nil && anon == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, signalsWriteTimeout)
	defer cancel()
	err := t.rec.RecordActivity(ctx, signalsclient.ActivitySpec{
		EventName:  name,
		Source:     source,
		ChannelID:  channel,
		CustomerID: cust,
		AnonID:     anon,
		At:         eventTime(ctx),
		Props:      props,
		DedupeKey:  dedupe,
	})
	if signalsclient.Unreachable(err) {
		return fmt.Errorf("record %s activity: %w", name, err)
	}
	return nil
}

// eventTime is the outbox enqueue stamp on a relayed dispatch, else now - stable across redeliveries, so a replayed event keeps the exact occurred_at (and bucketed dedupe key) of the first attempt and collapses in storage on both engines
func eventTime(ctx context.Context) time.Time {
	return events.EmittedAtOr(ctx, time.Now())
}

// Who an order's later events belong to - the customer, or for a guest the visitor id it was placed under
type orderRef struct {
	cust    *uuid.UUID
	anon    string
	number  string
	channel uuid.UUID
}

// A failed load goes back to the outbox like an unreachable suite - only a missing order is final
func (t *Tracker) resolveOrder(ctx context.Context, orderID uuid.UUID) (orderRef, error) {
	o, err := t.orders.GetByID(ctx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		return orderRef{}, nil
	}
	if err != nil {
		return orderRef{}, fmt.Errorf("load order %s: %w", orderID, err)
	}
	return orderRef{cust: o.CustomerID, anon: o.AnonID, number: o.Number, channel: o.ChannelID}, nil
}

func (t *Tracker) emitOrder(ctx context.Context, name string, o orderRef, dedupe string, props map[string]any) error {
	return t.emitID(ctx, sourceCommerce, name, o.channel, o.cust, o.anon, dedupe, props)
}

// bucketing collapses rapid repeats (cart churn, logins) into one dedupe key - derived from eventTime, not the wall clock, so a redelivery lands on the same key
func minuteBucket(ctx context.Context) int64 { return eventTime(ctx).Unix() / 60 }
func secBucket(ctx context.Context) int64    { return eventTime(ctx).Unix() }

func ref(id uuid.UUID) *uuid.UUID { return &id }

// --- commerce ---

func (t *Tracker) onOrderPlaced(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderPlaced)
	if !ok || (ev.CustomerID == nil && ev.AnonID == "") {
		return nil
	}
	if err := t.emitID(ctx, sourceCommerce, "order_placed", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:order_placed:"+ev.OrderID.String(),
		map[string]any{"order_id": ev.OrderID.String(), "order_number": ev.OrderNumber, "total_cents": ev.TotalCents, "currency": ev.Currency}); err != nil {
		return err
	}
	// gift-card-as-tender is applied during placement, surface it separately
	if o, err := t.orders.GetByID(ctx, ev.OrderID); err == nil && o.GiftCardAmount > 0 {
		return t.emitID(ctx, sourceCommerce, "giftcard_redeemed", ev.ChannelID, ev.CustomerID, ev.AnonID,
			"act:gc:"+ev.OrderID.String(),
			map[string]any{"order_number": ev.OrderNumber, "amount_cents": o.GiftCardAmount, "currency": ev.Currency})
	}
	return nil
}

func (t *Tracker) onOrderStatusChanged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderStatusChanged)
	if !ok {
		return nil
	}
	o, err := t.resolveOrder(ctx, ev.OrderID)
	if err != nil {
		return err
	}
	return t.emitOrder(ctx, "order_"+ev.ToStatus, o,
		"act:order_status:"+ev.ToStatus+":"+ev.OrderID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"order_id": ev.OrderID.String(), "order_number": o.number, "from": ev.FromStatus, "status": ev.ToStatus})
}

func (t *Tracker) onOrderPayment(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderPaymentStatusChanged)
	if !ok {
		return nil
	}
	o, err := t.resolveOrder(ctx, ev.OrderID)
	if err != nil {
		return err
	}
	return t.emitOrder(ctx, "order_payment_"+ev.Status, o,
		"act:order_pay:"+ev.Status+":"+ev.OrderID.String()+":"+ev.Reference,
		map[string]any{"order_number": o.number, "status": ev.Status, "reference": ev.Reference})
}

func (t *Tracker) onOrderShipped(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderFulfillmentCreated)
	if !ok {
		return nil
	}
	o, err := t.resolveOrder(ctx, ev.OrderID)
	if err != nil {
		return err
	}
	return t.emitOrder(ctx, "order_shipped", o,
		"act:order_shipped:"+ev.FulfillmentID.String(),
		map[string]any{"order_number": o.number, "fully_shipped": ev.FullyShipped})
}

func (t *Tracker) onReturnReceived(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderReturnReceived)
	if !ok {
		return nil
	}
	return t.recordReturn(ctx, "return_received", ev.OrderID, ev.ReturnID)
}

func (t *Tracker) onReturnRequested(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderReturnRequested)
	if !ok {
		return nil
	}
	return t.recordReturn(ctx, "return_requested", ev.OrderID, ev.ReturnID)
}

func (t *Tracker) onReturnApproved(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderReturnApproved)
	if !ok {
		return nil
	}
	return t.recordReturn(ctx, "return_approved", ev.OrderID, ev.ReturnID)
}

func (t *Tracker) onReturnRejected(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderReturnRejected)
	if !ok {
		return nil
	}
	return t.recordReturn(ctx, "return_rejected", ev.OrderID, ev.ReturnID)
}

func (t *Tracker) recordReturn(ctx context.Context, name string, orderID, returnID uuid.UUID) error {
	o, err := t.resolveOrder(ctx, orderID)
	if err != nil {
		return err
	}
	return t.emitOrder(ctx, name, o,
		"act:"+name+":"+returnID.String(),
		map[string]any{"order_number": o.number, "return_id": returnID.String()})
}

func (t *Tracker) onOrderEdited(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderEdited)
	if !ok {
		return nil
	}
	o, err := t.resolveOrder(ctx, ev.OrderID)
	if err != nil {
		return err
	}
	return t.emitOrder(ctx, "order_edited", o,
		"act:order_edited:"+ev.OrderID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"order_number": o.number, "new_amount_due": ev.NewAmountDue})
}

func (t *Tracker) onCartAdd(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineAdded)
	if !ok {
		return nil
	}
	return t.emitID(ctx, sourceCommerce, "cart_add", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:cart_add:"+ev.LineID.String(),
		map[string]any{"sku": ev.ProductSku, "qty": ev.Quantity, "unit_price": ev.UnitPrice})
}

func (t *Tracker) onCartBundleAdd(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.BundleAdded)
	if !ok {
		return nil
	}
	return t.emitID(ctx, sourceCommerce, "cart_add", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:cart_add:"+ev.LineID.String(),
		map[string]any{"sku": ev.ProductSku, "qty": ev.Quantity, "unit_price": ev.UnitPrice, "bundle": true})
}

func (t *Tracker) onCartUpdate(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineUpdated)
	if !ok {
		return nil
	}
	return t.emitID(ctx, sourceCommerce, "cart_update", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:cart_upd:"+ev.LineID.String()+":"+itoa(minuteBucket(ctx)),
		map[string]any{"line_id": ev.LineID.String(), "qty": ev.Quantity})
}

func (t *Tracker) onCartRemove(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineRemoved)
	if !ok {
		return nil
	}
	return t.emitID(ctx, sourceCommerce, "cart_remove", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:cart_rm:"+ev.LineID.String()+":"+itoa(minuteBucket(ctx)),
		map[string]any{"line_id": ev.LineID.String()})
}

func (t *Tracker) onCartCleared(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.Cleared)
	if !ok {
		return nil
	}
	return t.emitID(ctx, sourceCommerce, "cart_cleared", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:cart_clear:"+ev.CartID.String()+":"+itoa(minuteBucket(ctx)), nil)
}

func (t *Tracker) onCartMerged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.Merged)
	if !ok {
		return nil
	}
	return t.emit(ctx, sourceCommerce, "cart_merged", ev.ChannelID, ref(ev.CustomerID),
		"act:cart_merged:"+ev.CustomerCartID.String()+":"+ev.GuestCartID.String(),
		map[string]any{"guest_cart_id": ev.GuestCartID.String()})
}

func (t *Tracker) onReviewSubmitted(ctx context.Context, e events.Event) error {
	ev, ok := e.(*review.Submitted)
	if !ok {
		return nil
	}
	return t.emit(ctx, sourceCommerce, "review_submitted", ev.ChannelID, ev.CustomerID,
		"act:review:"+ev.ReviewID.String(),
		map[string]any{"product_id": ev.ProductID.String(), "status": string(ev.Status), "review_id": ev.ReviewID.String()})
}

func (t *Tracker) onSubLifecycle(ctx context.Context, e events.Event) error {
	ev, ok := e.(*subscription.Lifecycle)
	if !ok {
		return nil
	}
	verb := strings.TrimPrefix(ev.Kind, "subscription.")
	return t.emit(ctx, sourceCommerce, "subscription_"+verb, ev.ChannelID, ev.CustomerID,
		"act:sub_"+verb+":"+ev.SubscriptionID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"subscription_id": ev.SubscriptionID.String()})
}

func (t *Tracker) onSubInvoiced(ctx context.Context, e events.Event) error {
	ev, ok := e.(*subscription.Invoiced)
	if !ok {
		return nil
	}
	return t.emit(ctx, sourceCommerce, "subscription_invoiced", ev.ChannelID, ev.CustomerID,
		"act:sub_invoiced:"+ev.SubscriptionID.String()+":"+itoa(int64(ev.Sequence)),
		map[string]any{"subscription_id": ev.SubscriptionID.String(), "amount_cents": ev.AmountCents, "currency": ev.Currency})
}

func (t *Tracker) onDownloadAccessed(ctx context.Context, e events.Event) error {
	ev, ok := e.(*download.Accessed)
	if !ok {
		return nil
	}
	o := orderRef{cust: ev.CustomerID}
	if ev.OrderID != nil {
		var err error
		if o, err = t.resolveOrder(ctx, *ev.OrderID); err != nil {
			return err
		}
		if ev.CustomerID != nil {
			o.cust = ev.CustomerID
		}
	}
	// A grant issued without an order names no channel, so the storefront request that downloaded supplies it - and one with neither stays off the spine, where no per-channel report could show it
	if ch, ok := channel.FromContext(ctx); o.channel == uuid.Nil && ok && ch != nil {
		o.channel = ch.ID
	}
	if o.channel == uuid.Nil {
		return nil
	}
	return t.emitOrder(ctx, "download_accessed", o,
		"act:dl:"+ev.GrantID.String()+":"+itoa(minuteBucket(ctx)),
		map[string]any{"name": ev.Title, "order_number": o.number})
}

// --- account ---

func (t *Tracker) onRegistered(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerRegistered)
	if !ok {
		return nil
	}
	return t.emit(ctx, sourceAccount, "registered", ev.ChannelID, ref(ev.CustomerID),
		"act:registered:"+ev.CustomerID.String(),
		map[string]any{"source": ev.Source})
}

func (t *Tracker) onEmailVerified(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerEmailVerified)
	if !ok {
		return nil
	}
	return t.emit(ctx, sourceAccount, "email_verified", ev.ChannelID, ref(ev.CustomerID),
		"act:email_verified:"+ev.CustomerID.String(), nil)
}

func (t *Tracker) onAccountUpdated(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerUpdated)
	if !ok || ev.Source == customer.SourceGDPR {
		return nil // an erased customer's id must not land on the spine again
	}
	return t.emit(ctx, sourceAccount, "account_updated", ev.ChannelID, ref(ev.CustomerID),
		"act:account_updated:"+ev.CustomerID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"changed_fields": strings.Join(ev.ChangedFields, ","), "source": ev.Source})
}

func (t *Tracker) onProfileUpdated(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerProfileUpdated)
	if !ok {
		return nil
	}
	if ev.Source == customer.SourceImport {
		// An operator's bulk write, so it lands as an account change like an admin edit rather than as the shopper's own
		return t.emit(ctx, sourceAccount, "account_updated", ev.ChannelID, ref(ev.CustomerID),
			"act:account_updated:"+ev.CustomerID.String()+":"+itoa(secBucket(ctx)),
			map[string]any{"changed_fields": strings.Join(ev.ChangedFields, ","), "source": ev.Source})
	}
	return t.emit(ctx, sourceAccount, "profile_updated", ev.ChannelID, ref(ev.CustomerID),
		"act:profile_updated:"+ev.CustomerID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"changed_fields": strings.Join(ev.ChangedFields, ",")})
}

func (t *Tracker) onPasswordChanged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerPasswordChanged)
	if !ok {
		return nil
	}
	return t.emit(ctx, sourceAccount, "password_changed", ev.ChannelID, ref(ev.CustomerID),
		"act:pwchg:"+ev.CustomerID.String()+":"+itoa(secBucket(ctx)), nil)
}

func (t *Tracker) onAddressChanged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerAddressChanged)
	if !ok {
		return nil
	}
	return t.emit(ctx, sourceAccount, "address_changed", ev.ChannelID, ref(ev.CustomerID),
		"act:address:"+ev.AddressID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"action": ev.Action})
}

func (t *Tracker) onLogin(ctx context.Context, e events.Event) error {
	ev, ok := e.(*auth.LoginSucceeded)
	if !ok || ev.Subject != auth.SubjectCustomer {
		return nil
	}
	// IP is truncated - full IP + customer linkage is PII we don't need at rest
	var props map[string]any
	if ip := anonymizeIP(ev.IP); ip != "" {
		props = map[string]any{"ip": ip}
	}
	return t.emit(ctx, sourceAccount, "login", ev.ChannelID, ref(ev.SubjectID),
		"act:login:"+ev.SubjectID.String()+":"+itoa(secBucket(ctx)), props)
}

func (t *Tracker) onLogout(ctx context.Context, e events.Event) error {
	ev, ok := e.(*auth.Logout)
	if !ok || ev.Subject != auth.SubjectCustomer {
		return nil
	}
	// The event names no channel, so the storefront request that logged out supplies it
	chID := uuid.Nil
	if ch, ok := channel.FromContext(ctx); ok && ch != nil {
		chID = ch.ID
	}
	return t.emit(ctx, sourceAccount, "logout", chID, ref(ev.SubjectID),
		"act:logout:"+ev.SubjectID.String()+":"+itoa(secBucket(ctx)), nil) // always one session - revoke-all is a side effect of a password or account change, which records its own event
}

func (t *Tracker) onPasswordReset(ctx context.Context, e events.Event) error {
	ev, ok := e.(*auth.PasswordResetCompleted)
	if !ok || ev.Subject != auth.SubjectCustomer {
		return nil
	}
	return t.emit(ctx, sourceAccount, "password_reset", ev.ChannelID, ref(ev.SubjectID),
		"act:pwreset:"+ev.SubjectID.String()+":"+itoa(secBucket(ctx)), nil)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
