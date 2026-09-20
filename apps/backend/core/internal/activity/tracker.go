// Package activity subscribes to the domain event bus and turns every customer-facing action (commerce + account) into an append to the analytics spine, so the profile timeline reflects a customer's full journey
// not just web behaviour and email
package activity

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
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

func (t *Tracker) emit(ctx context.Context, source, name string, channel uuid.UUID, cust *uuid.UUID, dedupe string, props map[string]any) {
	t.emitID(ctx, source, name, channel, cust, "", dedupe, props)
}

// emitID also accepts an anon id for guest commerce - No-op when neither a
// customer nor an anon id is present
func (t *Tracker) emitID(ctx context.Context, source, name string, channel uuid.UUID, cust *uuid.UUID, anon, dedupe string, props map[string]any) {
	if cust == nil && anon == "" {
		return
	}
	_ = t.rec.RecordActivity(ctx, signalsclient.ActivitySpec{
		EventName:  name,
		Source:     source,
		ChannelID:  channel,
		CustomerID: cust,
		AnonID:     anon,
		At:         eventTime(ctx),
		Props:      props,
		DedupeKey:  dedupe,
	})
}

// eventTime is the outbox enqueue stamp on a relayed dispatch, else now - stable across redeliveries, so a replayed event keeps the exact occurred_at (and bucketed dedupe key) of the first attempt and collapses in storage on both engines
func eventTime(ctx context.Context) time.Time {
	return events.EmittedAtOr(ctx, time.Now())
}

// ok is false for guest orders (no customer) or load errors
func (t *Tracker) resolveOrder(ctx context.Context, orderID uuid.UUID) (cust *uuid.UUID, number string, channel uuid.UUID, ok bool) {
	o, err := t.orders.GetByID(ctx, orderID)
	if err != nil || o.CustomerID == nil {
		return nil, "", uuid.Nil, false
	}
	return o.CustomerID, o.Number, o.ChannelID, true
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
	t.emitID(ctx, sourceCommerce, "order_placed", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:order_placed:"+ev.OrderID.String(),
		map[string]any{"order_id": ev.OrderID.String(), "order_number": ev.OrderNumber, "total_cents": ev.TotalCents, "currency": ev.Currency})
	// gift-card-as-tender is applied during placement, surface it separately
	if o, err := t.orders.GetByID(ctx, ev.OrderID); err == nil && o.GiftCardAmount > 0 {
		t.emitID(ctx, sourceCommerce, "giftcard_redeemed", ev.ChannelID, ev.CustomerID, ev.AnonID,
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
	cust, number, ch, ok := t.resolveOrder(ctx, ev.OrderID)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceCommerce, "order_"+ev.ToStatus, ch, cust,
		"act:order_status:"+ev.ToStatus+":"+ev.OrderID.String(),
		map[string]any{"order_id": ev.OrderID.String(), "order_number": number, "from": ev.FromStatus, "status": ev.ToStatus})
	return nil
}

func (t *Tracker) onOrderPayment(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderPaymentStatusChanged)
	if !ok {
		return nil
	}
	cust, number, ch, ok := t.resolveOrder(ctx, ev.OrderID)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceCommerce, "order_payment_"+ev.Status, ch, cust,
		"act:order_pay:"+ev.Status+":"+ev.OrderID.String()+":"+ev.Reference,
		map[string]any{"order_number": number, "status": ev.Status, "reference": ev.Reference})
	return nil
}

func (t *Tracker) onOrderShipped(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderFulfillmentCreated)
	if !ok {
		return nil
	}
	cust, number, ch, ok := t.resolveOrder(ctx, ev.OrderID)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceCommerce, "order_shipped", ch, cust,
		"act:order_shipped:"+ev.FulfillmentID.String(),
		map[string]any{"order_number": number, "fully_shipped": ev.FullyShipped})
	return nil
}

func (t *Tracker) onReturnReceived(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderReturnReceived)
	if !ok {
		return nil
	}
	t.recordReturn(ctx, "return_received", ev.OrderID, ev.ReturnID)
	return nil
}

func (t *Tracker) onReturnRequested(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderReturnRequested)
	if !ok {
		return nil
	}
	t.recordReturn(ctx, "return_requested", ev.OrderID, ev.ReturnID)
	return nil
}

func (t *Tracker) onReturnApproved(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderReturnApproved)
	if !ok {
		return nil
	}
	t.recordReturn(ctx, "return_approved", ev.OrderID, ev.ReturnID)
	return nil
}

func (t *Tracker) onReturnRejected(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderReturnRejected)
	if !ok {
		return nil
	}
	t.recordReturn(ctx, "return_rejected", ev.OrderID, ev.ReturnID)
	return nil
}

func (t *Tracker) recordReturn(ctx context.Context, name string, orderID, returnID uuid.UUID) {
	cust, number, ch, ok := t.resolveOrder(ctx, orderID)
	if !ok {
		return
	}
	t.emit(ctx, sourceCommerce, name, ch, cust,
		"act:"+name+":"+returnID.String(),
		map[string]any{"order_number": number, "return_id": returnID.String()})
}

func (t *Tracker) onOrderEdited(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderEdited)
	if !ok {
		return nil
	}
	cust, number, ch, ok := t.resolveOrder(ctx, ev.OrderID)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceCommerce, "order_edited", ch, cust,
		"act:order_edited:"+ev.OrderID.String()+":"+itoa(ev.NewAmountDue),
		map[string]any{"order_number": number, "new_amount_due": ev.NewAmountDue})
	return nil
}

func (t *Tracker) onCartAdd(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineAdded)
	if !ok {
		return nil
	}
	t.emitID(ctx, sourceCommerce, "cart_add", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:cart_add:"+ev.LineID.String(),
		map[string]any{"sku": ev.ProductSku, "qty": ev.Quantity, "unit_price": ev.UnitPrice})
	return nil
}

func (t *Tracker) onCartBundleAdd(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.BundleAdded)
	if !ok {
		return nil
	}
	t.emitID(ctx, sourceCommerce, "cart_add", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:cart_add:"+ev.LineID.String(),
		map[string]any{"sku": ev.ProductSku, "qty": ev.Quantity, "unit_price": ev.UnitPrice, "bundle": true})
	return nil
}

func (t *Tracker) onCartUpdate(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineUpdated)
	if !ok {
		return nil
	}
	t.emitID(ctx, sourceCommerce, "cart_update", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:cart_upd:"+ev.LineID.String()+":"+itoa(minuteBucket(ctx)),
		map[string]any{"line_id": ev.LineID.String(), "qty": ev.Quantity})
	return nil
}

func (t *Tracker) onCartRemove(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineRemoved)
	if !ok {
		return nil
	}
	t.emitID(ctx, sourceCommerce, "cart_remove", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:cart_rm:"+ev.LineID.String()+":"+itoa(minuteBucket(ctx)),
		map[string]any{"line_id": ev.LineID.String()})
	return nil
}

func (t *Tracker) onCartCleared(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.Cleared)
	if !ok {
		return nil
	}
	t.emitID(ctx, sourceCommerce, "cart_cleared", ev.ChannelID, ev.CustomerID, ev.AnonID,
		"act:cart_clear:"+ev.CartID.String()+":"+itoa(minuteBucket(ctx)), nil)
	return nil
}

func (t *Tracker) onCartMerged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.Merged)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceCommerce, "cart_merged", ev.ChannelID, ref(ev.CustomerID),
		"act:cart_merged:"+ev.CustomerCartID.String()+":"+ev.GuestCartID.String(),
		map[string]any{"guest_cart_id": ev.GuestCartID.String()})
	return nil
}

func (t *Tracker) onReviewSubmitted(ctx context.Context, e events.Event) error {
	ev, ok := e.(*review.Submitted)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceCommerce, "review_submitted", ev.ChannelID, ev.CustomerID,
		"act:review:"+ev.ReviewID.String(),
		map[string]any{"product_id": ev.ProductID.String(), "status": string(ev.Status), "review_id": ev.ReviewID.String()})
	return nil
}

func (t *Tracker) onSubLifecycle(ctx context.Context, e events.Event) error {
	ev, ok := e.(*subscription.Lifecycle)
	if !ok {
		return nil
	}
	verb := strings.TrimPrefix(ev.Kind, "subscription.")
	t.emit(ctx, sourceCommerce, "subscription_"+verb, ev.ChannelID, ev.CustomerID,
		"act:sub_"+verb+":"+ev.SubscriptionID.String(),
		map[string]any{"subscription_id": ev.SubscriptionID.String()})
	return nil
}

func (t *Tracker) onSubInvoiced(ctx context.Context, e events.Event) error {
	ev, ok := e.(*subscription.Invoiced)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceCommerce, "subscription_invoiced", ev.ChannelID, ev.CustomerID,
		"act:sub_invoiced:"+ev.SubscriptionID.String()+":"+itoa(int64(ev.Sequence)),
		map[string]any{"subscription_id": ev.SubscriptionID.String(), "amount_cents": ev.AmountCents, "currency": ev.Currency})
	return nil
}

func (t *Tracker) onDownloadAccessed(ctx context.Context, e events.Event) error {
	ev, ok := e.(*download.Accessed)
	if !ok || ev.CustomerID == nil {
		return nil
	}
	var ch uuid.UUID
	number := ""
	if ev.OrderID != nil {
		if o, err := t.orders.GetByID(ctx, *ev.OrderID); err == nil {
			ch, number = o.ChannelID, o.Number
		}
	}
	t.emit(ctx, sourceCommerce, "download_accessed", ch, ev.CustomerID,
		"act:dl:"+ev.GrantID.String()+":"+itoa(minuteBucket(ctx)),
		map[string]any{"name": ev.Title, "order_number": number})
	return nil
}

// --- account ---

func (t *Tracker) onRegistered(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerRegistered)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceAccount, "registered", ev.ChannelID, ref(ev.CustomerID),
		"act:registered:"+ev.CustomerID.String(),
		map[string]any{"source": ev.Source})
	return nil
}

func (t *Tracker) onEmailVerified(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerEmailVerified)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceAccount, "email_verified", ev.ChannelID, ref(ev.CustomerID),
		"act:email_verified:"+ev.CustomerID.String(), nil)
	return nil
}

func (t *Tracker) onAccountUpdated(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerUpdated)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceAccount, "account_updated", ev.ChannelID, ref(ev.CustomerID),
		"act:account_updated:"+ev.CustomerID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"changed_fields": strings.Join(ev.ChangedFields, ","), "source": ev.Source})
	return nil
}

func (t *Tracker) onProfileUpdated(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerProfileUpdated)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceAccount, "profile_updated", ev.ChannelID, ref(ev.CustomerID),
		"act:profile_updated:"+ev.CustomerID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"changed_fields": strings.Join(ev.ChangedFields, ",")})
	return nil
}

func (t *Tracker) onPasswordChanged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerPasswordChanged)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceAccount, "password_changed", ev.ChannelID, ref(ev.CustomerID),
		"act:pwchg:"+ev.CustomerID.String()+":"+itoa(secBucket(ctx)), nil)
	return nil
}

func (t *Tracker) onAddressChanged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerAddressChanged)
	if !ok {
		return nil
	}
	t.emit(ctx, sourceAccount, "address_changed", ev.ChannelID, ref(ev.CustomerID),
		"act:address:"+ev.AddressID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"action": ev.Action})
	return nil
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
	t.emit(ctx, sourceAccount, "login", ev.ChannelID, ref(ev.SubjectID),
		"act:login:"+ev.SubjectID.String()+":"+itoa(secBucket(ctx)), props)
	return nil
}

func (t *Tracker) onLogout(ctx context.Context, e events.Event) error {
	ev, ok := e.(*auth.Logout)
	if !ok || ev.Subject != auth.SubjectCustomer {
		return nil
	}
	t.emit(ctx, sourceAccount, "logout", uuid.Nil, ref(ev.SubjectID),
		"act:logout:"+ev.SubjectID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"all_devices": ev.AllDevices})
	return nil
}

func (t *Tracker) onPasswordReset(ctx context.Context, e events.Event) error {
	ev, ok := e.(*auth.PasswordResetCompleted)
	if !ok || ev.Subject != auth.SubjectCustomer {
		return nil
	}
	t.emit(ctx, sourceAccount, "password_reset", ev.ChannelID, ref(ev.SubjectID),
		"act:pwreset:"+ev.SubjectID.String()+":"+itoa(secBucket(ctx)), nil)
	return nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
