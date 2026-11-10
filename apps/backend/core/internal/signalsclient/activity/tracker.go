// Package activity subscribes to the domain event bus and turns every customer-facing action (commerce + account) into an append to the analytics spine, so the profile timeline reflects a customer's full journey
// not just web behaviour and email
package activity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/signupform"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/wishlist"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// Satisfied by *signalsclient.Client - an interface so the tracker tests without a service
type activityRecorder interface {
	RecordActivity(ctx context.Context, a signalsclient.ActivitySpec) error
	RecordAssistantTurn(ctx context.Context, t *signalsv1.AssistantTurnRecord) error
	RecordAIActivity(ctx context.Context, a signalsclient.AIActivitySpec) error
}

// Each SKU's product and the products it's sold through, so a cart or an order names what the assistant showed
type productRefs interface {
	ProductRefsBySKU(ctx context.Context, skus []string) ([]db.ProductRefsBySKURow, error)
}

// The spine's source column for what this package emits - core writes only these two
const (
	sourceCommerce  = "commerce"
	sourceAccount   = "account"
	sourceAggregate = "aggregate" // counted as nobody - the suite keeps it out of every per-person read
)

const signalsWriteTimeout = 2 * time.Second

// Satisfied by *order.Service - an order, and the lines a refund on it credited
type orderLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
	GetCreditMemo(ctx context.Context, id uuid.UUID) (order.CreditMemo, error)
}

// Satisfied by *currency.BaseRates - amounts stamped in the base currency as they happen
type baseRates interface {
	From(ctx context.Context, code string) currency.ToBase
}

// Settings reads a shop's switches - satisfied by *config.Service
type Settings interface {
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
}

// AggregateCountsSetting is whether a shop counts visitors who decline analytics, as nobody
const AggregateCountsSetting = "general/privacy/aggregate_counts"

type Tracker struct {
	rec      activityRecorder
	orders   orderLookup
	settings Settings
	products productRefs
	base     baseRates
}

// NewTracker needs the shop's settings: whether a visitor who declined analytics is counted at all is the shop's call,
// and every path here - the live events, a backfill, a counted page view - has to follow it
func NewTracker(rec activityRecorder, orders orderLookup, settings Settings, products productRefs, base baseRates) *Tracker {
	return &Tracker{rec: rec, orders: orders, settings: settings, products: products, base: base}
}

// countsNobody is the shop's aggregate_counts switch - a setting that won't read keeps its default, on
func (t *Tracker) countsNobody(ctx context.Context, channelID uuid.UUID) bool {
	on, err := t.settings.GetBool(ctx, AggregateCountsSetting, config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID})
	return err != nil || on
}

// CountView counts a page view from a visitor who declined analytics, as nobody - the path alone, every view its own
func (t *Tracker) CountView(ctx context.Context, channelID uuid.UUID, path string) error {
	return t.emitAs(ctx, sourceCommerce, "page_view", channelID, actor{nobody: true}, "agg:"+uuid.NewString(), map[string]any{"path": path})
}

// BackfillOrder replays a past order's placement, for orders that never reached the suite - placedAt must be the time the
// live event carried, since the dedupe key only collapses a copy in the same daily partition
// An order placed without analytics consent is skipped: it already counted, under an id of its own before such orders
// counted as nobody and as nobody since, and the key it would get now matches neither, so a replay would count it twice
func (t *Tracker) BackfillOrder(ctx context.Context, o order.Order, placedAt time.Time) error {
	if orderActor(o).nobody {
		return nil
	}
	ctx = events.WithEmittedAt(ctx, placedAt)
	return t.recordPlaced(ctx, &order.OrderPlaced{
		OrderID: o.ID, OrderNumber: o.Number, CustomerID: o.CustomerID, Email: o.Email, AnonID: o.AnonID, AssistedBy: o.AssistedBy,
		ChannelID: o.ChannelID, TotalCents: o.TotalAmount, Currency: o.CurrencyCode,
	}, o)
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
	bus.Subscribe(order.EventRefunded, t.onOrderRefunded)
	bus.Subscribe(cart.EventLineAdded, t.onCartAdd)
	bus.Subscribe(cart.EventBundleAdded, t.onCartBundleAdd)
	bus.Subscribe(cart.EventLineUpdated, t.onCartUpdate)
	bus.Subscribe(cart.EventLineRemoved, t.onCartRemove)
	bus.Subscribe(cart.EventCleared, t.onCartCleared)
	bus.Subscribe(cart.EventMerged, t.onCartMerged)
	bus.Subscribe(cart.EventCheckoutChosen, t.onCheckoutChosen)
	bus.Subscribe(wishlist.EventItemAdded, t.onWishlistAdd)
	bus.Subscribe(wishlist.EventItemRemoved, t.onWishlistRemove)
	bus.Subscribe(signupform.EventSubmitted, t.onLead)
	bus.Subscribe(review.EventSubmitted, t.onReviewSubmitted)
	bus.Subscribe(shoptools.EventAssistantTurn, t.onAssistantTurn)
	bus.Subscribe(shoptools.EventAgentCall, t.onAgentCall)
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

// actor is who an event is about: a customer, a visitor id, or nobody - a visitor who declined analytics, counted in
// totals and revenue and never on a profile
type actor struct {
	cust   *uuid.UUID
	anon   string
	nobody bool
}

// shopperActor is a shopper's own action: theirs, or nobody when they refused analytics
func shopperActor(s *auth.Shopper, cust *uuid.UUID, anon string) actor {
	if s.AnalyticsRefused() {
		return actor{nobody: true}
	}
	return actor{cust: cust, anon: anon}
}

// orderActor is who an order's events belong to - nobody when it was placed without analytics consent (the snapshot
// is the consent at purchase), else the customer or the guest it was placed as
func orderActor(o order.Order) actor {
	if o.Shopper.AnalyticsRefused() {
		return actor{nobody: true}
	}
	return actor{cust: o.CustomerID, anon: guestVisitor(o.CustomerID, o.AnonID, o.Email)}
}

// signInActor is the customer signing in or up, with the visitor id they arrived on when analytics allows - the suite
// stitches the two on a login or registered event. ShopperFromContext only reads the visitor id with that consent
func signInActor(s *auth.Shopper, cust uuid.UUID) actor {
	a := actor{cust: ref(cust)}
	if s != nil {
		a.anon = s.AnonID
	}
	return a
}

// emit is an account action, which is always the customer's own
func (t *Tracker) emit(ctx context.Context, source, name string, channel uuid.UUID, cust *uuid.UUID, dedupe string, props map[string]any) error {
	return t.emitAs(ctx, source, name, channel, actor{cust: cust}, dedupe, props)
}

// emitAs records one event for who it is about. Nobody goes as an aggregate event, only where the shop counts them:
// no customer and no visitor id, and the suite keeps only amounts and items, the hour, and a keyed hash of the dedupe
// key, so nothing it holds leads back to the order or line. An event with no one at all has no place on the spine
// An unreachable suite hands the event back to the outbox, whose replay the dedupe key collapses - a refusal is final
func (t *Tracker) emitAs(ctx context.Context, source, name string, channel uuid.UUID, who actor, dedupe string, props map[string]any) error {
	spec := signalsclient.ActivitySpec{EventName: name, Source: source, ChannelID: channel, At: eventTime(ctx), Props: props, DedupeKey: dedupe}
	switch {
	case who.nobody:
		if !t.countsNobody(ctx, channel) {
			return nil
		}
		spec.Source = sourceAggregate
	case who.cust == nil && who.anon == "":
		return nil
	default:
		spec.CustomerID, spec.AnonID = who.cust, who.anon
	}
	return t.record(ctx, spec)
}

func (t *Tracker) record(ctx context.Context, spec signalsclient.ActivitySpec) error {
	ctx, cancel := context.WithTimeout(ctx, signalsWriteTimeout)
	defer cancel()
	err := t.rec.RecordActivity(ctx, spec)
	if signalsclient.Unreachable(err) {
		return fmt.Errorf("record %s activity: %w", spec.EventName, err)
	}
	if err != nil {
		slog.Warn("signals refused an activity write (dropped)", "event", spec.EventName, "err", err)
	}
	return nil
}

// eventTime is the outbox enqueue stamp on a relayed dispatch, else now - stable across redeliveries, so a replayed event keeps the exact occurred_at (and bucketed dedupe key) of the first attempt and collapses in storage on both engines
func eventTime(ctx context.Context) time.Time {
	return events.EmittedAtOr(ctx, time.Now())
}

// An order's later events: who they belong to, and the order they name
type orderRef struct {
	who     actor
	number  string
	channel uuid.UUID
}

// A failed load goes back to the outbox like an unreachable suite - only a missing order is final, and found says so
func (t *Tracker) loadOrder(ctx context.Context, orderID uuid.UUID) (o order.Order, found bool, err error) {
	o, err = t.orders.GetByID(ctx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		return order.Order{}, false, nil
	}
	if err != nil {
		return order.Order{}, false, fmt.Errorf("load order %s: %w", orderID, err)
	}
	return o, true, nil
}

func refOf(o order.Order) orderRef {
	return orderRef{who: orderActor(o), number: o.Number, channel: o.ChannelID}
}

// resolveOrder is who a later event on an order belongs to - the zero ref, which records nothing, for a missing order
func (t *Tracker) resolveOrder(ctx context.Context, orderID uuid.UUID) (orderRef, error) {
	o, found, err := t.loadOrder(ctx, orderID)
	if err != nil || !found {
		return orderRef{}, err
	}
	return refOf(o), nil
}

// guestVisitor is who a guest order belongs to: the storefront visitor when one was sent, else a hash of the receipt
// email, so admin drafts, headless shops and API orders still land and one guest's repeat orders read as one person - an
// erased guest's scrubbed address names nobody, so their orders stay off the spine
func guestVisitor(cust *uuid.UUID, anon, email string) string {
	if cust != nil || anon != "" {
		return anon
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || strings.HasSuffix(email, customer.RedactedEmailDomain) {
		return ""
	}
	sum := sha256.Sum256([]byte(email))
	return "guest_" + hex.EncodeToString(sum[:16])
}

// bucketing collapses rapid repeats (cart churn, logins) into one dedupe key - derived from eventTime, not the wall clock, so a redelivery lands on the same key
func minuteBucket(ctx context.Context) int64 { return eventTime(ctx).Unix() / 60 }
func secBucket(ctx context.Context) int64    { return eventTime(ctx).Unix() }

func ref(id uuid.UUID) *uuid.UUID { return &id }

// --- commerce ---

func (t *Tracker) onOrderPlaced(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderPlaced)
	if !ok {
		return nil
	}
	// Loaded first for the consent snapshot, the lines and the products they name, and the gift-card tender
	o, found, err := t.loadOrder(ctx, ev.OrderID)
	if err != nil || !found {
		return err
	}
	return t.recordPlaced(ctx, ev, o)
}

// recordPlaced is an order's placement and what follows from it, for the order as it was placed
func (t *Tracker) recordPlaced(ctx context.Context, ev *order.OrderPlaced, o order.Order) error {
	placed := orderRef{who: orderActor(o), number: ev.OrderNumber, channel: ev.ChannelID}
	props := map[string]any{"order_id": ev.OrderID.String(), "order_number": ev.OrderNumber, "total_cents": ev.TotalCents, "currency": ev.Currency}
	if ev.AssistedBy != "" {
		props["assisted_by"] = ev.AssistedBy
	}
	skus := make([]string, 0, len(o.Lines))
	for _, l := range o.Lines {
		skus = append(skus, l.ProductSku)
	}
	if ids := t.productIDs(ctx, skus...); len(ids) > 0 {
		props["product_ids"] = ids
	}
	if b, ok := t.base.From(ctx, ev.Currency)(ev.TotalCents); ok {
		props["total_base_cents"] = b
	}
	if err := t.emitAs(ctx, sourceCommerce, signalsapi.EventOrderPlaced, placed.channel, placed.who, "act:order_placed:"+ev.OrderID.String(), props); err != nil {
		return err
	}
	if ev.AssistedBy != "" {
		if err := t.emitAs(ctx, sourceCommerce, signalsapi.EventAssistedOrder, placed.channel, placed.who, "act:assisted_order:"+ev.OrderID.String(),
			map[string]any{"order_id": ev.OrderID.String(), "assisted_by": ev.AssistedBy, "total_cents": ev.TotalCents, "currency": ev.Currency}); err != nil {
			return err
		}
	}
	if err := t.recordPlacedLines(ctx, placed, o); err != nil {
		return err
	}
	if o.GiftCardAmount > 0 {
		return t.emitAs(ctx, sourceCommerce, "giftcard_redeemed", placed.channel, placed.who, "act:gc:"+ev.OrderID.String(),
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
	return t.emitAs(ctx, sourceCommerce, "order_"+ev.ToStatus, o.channel, o.who,
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
	return t.emitAs(ctx, sourceCommerce, "order_payment_"+ev.Status, o.channel, o.who,
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
	return t.emitAs(ctx, sourceCommerce, "order_shipped", o.channel, o.who,
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
	return t.emitAs(ctx, sourceCommerce, name, o.channel, o.who,
		"act:"+name+":"+returnID.String(),
		map[string]any{"order_id": orderID.String(), "order_number": o.number, "return_id": returnID.String()})
}

func (t *Tracker) onOrderRefunded(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderRefunded)
	if !ok {
		return nil
	}
	o, found, err := t.loadOrder(ctx, ev.OrderID)
	if err != nil || !found {
		return err
	}
	ref := refOf(o)
	if err := t.emitAs(ctx, sourceCommerce, "order_refunded", ref.channel, ref.who,
		"act:order_refunded:"+ev.CreditMemoID.String(),
		map[string]any{"order_number": ref.number, "credit_memo_id": ev.CreditMemoID.String(), "fully_refunded": ev.FullyRefunded}); err != nil {
		return err
	}
	memo, err := t.orders.GetCreditMemo(ctx, ev.CreditMemoID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load credit memo %s: %w", ev.CreditMemoID, err)
	}
	return t.recordRefundLines(ctx, ref, o, memo)
}

func (t *Tracker) onOrderEdited(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderEdited)
	if !ok {
		return nil
	}
	o, found, err := t.loadOrder(ctx, ev.OrderID)
	if err != nil || !found {
		return err
	}
	ref := refOf(o)
	if err := t.emitAs(ctx, sourceCommerce, "order_edited", ref.channel, ref.who,
		"act:order_edited:"+ev.OrderID.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"order_number": ref.number, "new_amount_due": ev.NewAmountDue}); err != nil {
		return err
	}
	if len(ev.Before) == 0 {
		return nil // an address edit moves no line
	}
	return t.recordLineEdits(ctx, ref, o, ev.Before)
}

func (t *Tracker) onCartAdd(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineAdded)
	if !ok {
		return nil
	}
	return t.emitAs(ctx, sourceCommerce, "cart_add", ev.ChannelID, shopperActor(ev.Shopper, ev.CustomerID, ev.AnonID),
		"act:cart_add:"+ev.LineID.String(),
		t.withProducts(ctx, ev.ProductSku, map[string]any{"sku": ev.ProductSku, "qty": ev.Quantity, "unit_price": ev.UnitPrice}))
}

func (t *Tracker) onCartBundleAdd(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.BundleAdded)
	if !ok {
		return nil
	}
	return t.emitAs(ctx, sourceCommerce, "cart_add", ev.ChannelID, shopperActor(ev.Shopper, ev.CustomerID, ev.AnonID),
		"act:cart_add:"+ev.LineID.String(),
		t.withProducts(ctx, ev.ProductSku, map[string]any{"sku": ev.ProductSku, "qty": ev.Quantity, "unit_price": ev.UnitPrice, "bundle": true}))
}

func (t *Tracker) withProducts(ctx context.Context, sku string, props map[string]any) map[string]any {
	if ids := t.productIDs(ctx, sku); len(ids) > 0 {
		props["product_ids"] = ids
	}
	return props
}

// productIDs is every product the SKUs are and are sold through, so the analytics can tell an order of a variant from
// the product the assistant showed - a failed lookup leaves them off rather than hold the event back
func (t *Tracker) productIDs(ctx context.Context, skus ...string) []string {
	if t.products == nil || len(skus) == 0 {
		return nil
	}
	rows, err := t.products.ProductRefsBySKU(ctx, skus)
	if err != nil {
		slog.WarnContext(ctx, "activity: reading the products behind SKUs failed", "err", err)
		return nil
	}
	seen := map[uuid.UUID]bool{}
	var out []string
	add := func(id uuid.UUID) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id.String())
		}
	}
	for _, r := range rows {
		add(r.ID)
		if r.ParentID.Valid {
			add(r.ParentID.UUID)
		}
	}
	return out
}

func (t *Tracker) onCartUpdate(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineUpdated)
	if !ok {
		return nil
	}
	return t.emitAs(ctx, sourceCommerce, "cart_update", ev.ChannelID, shopperActor(ev.Shopper, ev.CustomerID, ev.AnonID),
		"act:cart_upd:"+ev.LineID.String()+":"+itoa(minuteBucket(ctx)),
		map[string]any{"line_id": ev.LineID.String(), "sku": ev.ProductSku, "qty": ev.Quantity})
}

func (t *Tracker) onCartRemove(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineRemoved)
	if !ok {
		return nil
	}
	return t.emitAs(ctx, sourceCommerce, "cart_remove", ev.ChannelID, shopperActor(ev.Shopper, ev.CustomerID, ev.AnonID),
		"act:cart_rm:"+ev.LineID.String()+":"+itoa(minuteBucket(ctx)),
		map[string]any{"line_id": ev.LineID.String(), "sku": ev.ProductSku, "qty": ev.Quantity})
}

// checkout_shipping and checkout_payment, each with what was chosen when it changed
func (t *Tracker) onCheckoutChosen(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.CheckoutChosen)
	if !ok {
		return nil
	}
	who := shopperActor(ev.Shopper, ev.CustomerID, ev.AnonID)
	for name, choice := range map[string]string{"checkout_shipping": ev.Shipping, "checkout_payment": ev.Payment} {
		if choice == "" {
			continue
		}
		if err := t.emitAs(ctx, sourceCommerce, name, ev.ChannelID, who,
			"act:"+name+":"+ev.CartID.String()+":"+itoa(secBucket(ctx)), map[string]any{"choice": choice}); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tracker) onWishlistAdd(ctx context.Context, e events.Event) error {
	ev, ok := e.(*wishlist.ItemAdded)
	if !ok || ev.ChannelID == uuid.Nil {
		return nil
	}
	return t.wishlist(ctx, "wishlist_add", ev.ChannelID, ev.CustomerID, ev.ProductID, ev.Shopper)
}

func (t *Tracker) onWishlistRemove(ctx context.Context, e events.Event) error {
	ev, ok := e.(*wishlist.ItemRemoved)
	if !ok || ev.ChannelID == uuid.Nil {
		return nil
	}
	return t.wishlist(ctx, "wishlist_remove", ev.ChannelID, ev.CustomerID, ev.ProductID, ev.Shopper)
}

func (t *Tracker) wishlist(ctx context.Context, name string, channel, cust, product uuid.UUID, s *auth.Shopper) error {
	return t.emitAs(ctx, sourceCommerce, name, channel, shopperActor(s, &cust, visitorOf(s)),
		"act:"+name+":"+cust.String()+":"+product.String()+":"+itoa(secBucket(ctx)),
		map[string]any{"product_id": product.String()})
}

// A signup form sent - the visitor's, with no address, which isn't confirmed yet
func (t *Tracker) onLead(ctx context.Context, e events.Event) error {
	ev, ok := e.(*signupform.Submitted)
	if !ok {
		return nil
	}
	return t.emitAs(ctx, sourceCommerce, "lead", ev.ChannelID, shopperActor(ev.Shopper, nil, visitorOf(ev.Shopper)),
		"act:lead:"+ev.LeadKey, map[string]any{"form_id": ev.FormID.String()})
}

// visitorOf is the visitor id a shopper carries - only there with analytics consent
func visitorOf(s *auth.Shopper) string {
	if s == nil {
		return ""
	}
	return s.AnonID
}

func (t *Tracker) onCartCleared(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.Cleared)
	if !ok {
		return nil
	}
	return t.emitAs(ctx, sourceCommerce, "cart_cleared", ev.ChannelID, shopperActor(ev.Shopper, ev.CustomerID, ev.AnonID),
		"act:cart_clear:"+ev.CartID.String()+":"+itoa(minuteBucket(ctx)), nil)
}

func (t *Tracker) onCartMerged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.Merged)
	if !ok || ev.Shopper.AnalyticsRefused() {
		return nil
	}
	return t.emit(ctx, sourceCommerce, "cart_merged", ev.ChannelID, ref(ev.CustomerID),
		"act:cart_merged:"+ev.CustomerCartID.String()+":"+ev.GuestCartID.String(),
		map[string]any{"guest_cart_id": ev.GuestCartID.String()})
}

func (t *Tracker) onAssistantTurn(ctx context.Context, e events.Event) error {
	ev, ok := e.(*shoptools.AssistantTurn)
	if !ok {
		return nil
	}
	props := map[string]any{"products": ev.Products, "cart_changed": ev.CartChanged, "checkout": ev.Checkout, "incomplete": ev.Incomplete}
	if len(ev.ShownProducts) > 0 {
		ids := make([]string, len(ev.ShownProducts))
		for i, id := range ev.ShownProducts {
			ids[i] = id.String()
		}
		props["product_ids"] = ids
	}
	if err := t.emitAs(ctx, sourceCommerce, signalsapi.EventAssistantTurn, ev.ChannelID, shopperActor(ev.Shopper, ev.CustomerID, ev.AnonID), "act:assistant:"+ev.ID.String(), props); err != nil {
		return err
	}
	if err := t.aiActivity(ctx, signalsclient.AIActivitySpec{ChannelID: ev.ChannelID, Source: signalsapi.AISourceAssistant,
		Tools: ev.Tools, Searches: aiSearches(ev.Searches), DedupeKey: "ai:assistant:" + ev.ID.String()}); err != nil {
		return err
	}
	if ev.Transcript == nil || (ev.CustomerID == nil && ev.AnonID == "") {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, signalsWriteTimeout)
	defer cancel()
	err := t.rec.RecordAssistantTurn(ctx, transcriptRecord(ctx, ev))
	if signalsclient.Unreachable(err) {
		return fmt.Errorf("record assistant transcript: %w", err)
	}
	if err != nil {
		slog.Warn("signals refused an assistant transcript (dropped)", "err", err)
	}
	return nil
}

// A shopper's own agent's call - a bus event, so a suite that's down misses it rather than hold anything up
func (t *Tracker) onAgentCall(ctx context.Context, e events.Event) error {
	ev, ok := e.(*shoptools.AgentCall)
	if !ok {
		return nil
	}
	err := t.aiActivity(ctx, signalsclient.AIActivitySpec{ChannelID: ev.ChannelID, Source: signalsapi.AISourceAgent, Agent: ev.Agent,
		Tools: []string{ev.Tool}, Searches: aiSearches(ev.Searches), DedupeKey: "ai:agent:" + ev.ID.String()})
	if err != nil {
		slog.WarnContext(ctx, "activity: an agent's call didn't reach Signals", "err", err)
	}
	return nil
}

func (t *Tracker) aiActivity(ctx context.Context, a signalsclient.AIActivitySpec) error {
	if len(a.Tools) == 0 && len(a.Searches) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, signalsWriteTimeout)
	defer cancel()
	a.At = eventTime(ctx)
	err := t.rec.RecordAIActivity(ctx, a)
	if signalsclient.Unreachable(err) {
		return fmt.Errorf("record AI activity: %w", err)
	}
	if err != nil {
		slog.Warn("signals refused AI activity (dropped)", "source", a.Source, "err", err)
	}
	return nil
}

func aiSearches(in []shoptools.Search) []signalsclient.AISearch {
	out := make([]signalsclient.AISearch, len(in))
	for i, s := range in {
		out[i] = signalsclient.AISearch{Term: s.Term, Results: s.Results}
	}
	return out
}

func transcriptRecord(ctx context.Context, ev *shoptools.AssistantTurn) *signalsv1.AssistantTurnRecord {
	tr := ev.Transcript
	rec := &signalsv1.AssistantTurnRecord{
		Id: ev.ID.String(), ConversationId: tr.Conversation.String(), ChannelId: ev.ChannelID.String(), AnonId: ev.AnonID,
		Page: tr.Page, Question: tr.Question, Reply: tr.Reply, Recap: tr.Recap, FollowUps: tr.FollowUps, Products: int32(ev.Products),
		CartChanged: ev.CartChanged, Checkout: ev.Checkout, Incomplete: ev.Incomplete, FoundNothing: ev.FoundNothing, Unsure: ev.Unsure, Model: tr.Model,
		InputTokens: tr.Usage.Input, OutputTokens: tr.Usage.Output, CacheReadTokens: tr.Usage.CacheRead, CacheWriteTokens: tr.Usage.CacheWrite,
		At: timestamppb.New(eventTime(ctx)), Rounds: make([]*signalsv1.AssistantRound, len(tr.Rounds)),
	}
	if ev.CustomerID != nil {
		id := ev.CustomerID.String()
		rec.CustomerId = &id
	}
	for i, r := range tr.Rounds {
		rec.Rounds[i] = &signalsv1.AssistantRound{Thinking: r.Thinking, Text: r.Text, Shown: r.Shown, Calls: make([]*signalsv1.AssistantToolCall, len(r.Calls))}
		for j, c := range r.Calls {
			rec.Rounds[i].Calls[j] = &signalsv1.AssistantToolCall{Tool: c.Tool, InputJson: string(c.Input), Output: c.Output, IsError: c.IsError, Skipped: c.Skipped}
		}
	}
	signalsapi.FitAssistantTurn(rec)
	return rec
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
	o := orderRef{who: actor{cust: ev.CustomerID}}
	if ev.OrderID != nil {
		var err error
		if o, err = t.resolveOrder(ctx, *ev.OrderID); err != nil {
			return err
		}
		if ev.CustomerID != nil && !o.who.nobody { // a signed-in download is theirs, unless the order was bought as nobody
			o.who.cust = ev.CustomerID
		}
	}
	// A grant issued without an order names no channel, so the storefront request that downloaded supplies it - and one with neither stays off the spine, where no per-channel report could show it
	if o.channel == uuid.Nil {
		o.channel = channel.IDFromContext(ctx)
	}
	if o.channel == uuid.Nil {
		return nil
	}
	return t.emitAs(ctx, sourceCommerce, "download_accessed", o.channel, o.who,
		"act:dl:"+ev.GrantID.String()+":"+itoa(minuteBucket(ctx)),
		map[string]any{"name": ev.Title, "order_number": o.number})
}

// --- account ---

func (t *Tracker) onRegistered(ctx context.Context, e events.Event) error {
	ev, ok := e.(*customer.CustomerRegistered)
	if !ok {
		return nil
	}
	return t.emitAs(ctx, sourceAccount, "registered", ev.ChannelID, signInActor(ev.Shopper, ev.CustomerID),
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
	return t.emitAs(ctx, sourceAccount, "login", ev.ChannelID, signInActor(ev.Shopper, ev.SubjectID),
		"act:login:"+ev.SubjectID.String()+":"+itoa(secBucket(ctx)), props)
}

func (t *Tracker) onLogout(ctx context.Context, e events.Event) error {
	ev, ok := e.(*auth.Logout)
	if !ok || ev.Subject != auth.SubjectCustomer {
		return nil
	}
	// The event names no channel, so the storefront request that logged out supplies it
	return t.emit(ctx, sourceAccount, "logout", channel.IDFromContext(ctx), ref(ev.SubjectID),
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
