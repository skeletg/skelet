package wiring

import (
	"context"
	"log"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/internal/activity"
	"github.com/skeletg/skelet/apps/backend/internal/ai"
	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/internal/cacheinvalidate"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/download"
	"github.com/skeletg/skelet/apps/backend/internal/email"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/flow"
	"github.com/skeletg/skelet/apps/backend/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/internal/loyalty"
	"github.com/skeletg/skelet/apps/backend/internal/notification"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/ordercapture"
	"github.com/skeletg/skelet/apps/backend/internal/outbox"
	"github.com/skeletg/skelet/apps/backend/internal/payment"
	"github.com/skeletg/skelet/apps/backend/internal/referral"
	"github.com/skeletg/skelet/apps/backend/internal/revalidate"
	"github.com/skeletg/skelet/apps/backend/internal/review"
	"github.com/skeletg/skelet/apps/backend/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/internal/webform"
	"github.com/skeletg/skelet/apps/backend/internal/webhook"
)

// The one call the entrypoint makes - every "what reacts when X fires" decision is below it, and registration order is load-bearing where sync priorities tie, so treat the sequence here as part of the contract
type subscriberDeps struct {
	fx.In
	Bus               *events.Bus
	Outbox            *outbox.Outbox
	WebhookDispatcher *webhook.Dispatcher
	Order             *order.Service
	Payment           *payment.Service
	RevalidateCli     *revalidate.Client
	RevalidateQueue   *revalidate.Queue
	URLRewrite        *urlrewrite.Service
	Config            *config.Service
	Flow              *flow.Service
	Notification      *notification.Service
	Review            *review.Service
	AI                *ai.Service
	Email             email.Sender
	EmailTemplate     *emailtemplate.Service
	Recorder          *analytics.Recorder
	Signals           SignalsEnabled
	BackInStock       *backinstock.Service

	Inventory    *inventory.Service // the order reactors - each subscribes itself, this only decides that they all do and in what order
	Giftcard     *giftcard.Service
	StoreCredit  *storecredit.Service
	Loyalty      *loyalty.Service
	Referral     *referral.Service
	Download     *download.Service
	Subscription *subscription.Service
}

func RegisterDomainSubscribers(d subscriberDeps) {
	bus := d.Bus
	order.RegisterOutboxTypes(d.Outbox) // decoders first - the relay can't replay a type it can't materialize
	payment.RegisterOutboxTypes(d.Outbox)
	d.Payment.SetOutbox(d.Outbox)                                                                       // gateway webhook events become durable - a 204 to Stripe means "will be applied", not "was published"
	d.Outbox.SetDropAlert(func(ctx context.Context, eventType string, rowID uuid.UUID, reason string) { // a dropped row means a domain reaction (points, credit, capture) silently never ran - that needs an operator, not a log line
		if err := d.Notification.Create(ctx, notification.Input{
			Category:           notification.CategorySystem,
			Severity:           notification.SeverityCritical,
			Title:              "Event delivery gave up: " + eventType,
			Body:               "An outbox event exhausted its retries and was dropped: " + reason,
			RequiredPermission: notification.PermSystem,
			EntityType:         "outbox_event",
			EntityID:           rowID.String(),
			DedupeKey:          "outbox_drop:" + rowID.String(),
		}); err != nil {
			log.Printf("[outbox] drop alert for %s (%s): %v", eventType, rowID, err)
		}
	})

	registerEventSubscribers(bus, d.WebhookDispatcher, d.Order)
	subscribeOrderReactors(d)
	ordercapture.Register(bus, d.Order, d.Payment)
	cacheinvalidate.Register(bus, d.RevalidateCli, d.RevalidateQueue, d.URLRewrite, d.Config)
	flow.RegisterEnrollment(bus, d.Flow, d.Order)
	notification.NewNotifier(d.Notification).Register(bus) // admin inbox - domain events become store-ops notifications, live-pushed to dashboards over Valkey pub/sub
	review.RegisterSubscribers(bus, d.Review, d.AI.Enabled())
	webform.RegisterSubmitEmails(bus, d.Email)
	registerChannelSeed(bus, d.EmailTemplate, d.Flow)
	activity.RegisterConversionAttribution(bus, d.Recorder)

	if !d.Signals {
		return // the Recorder no-ops anyway - skipping the subscribe also skips their per-event order lookups
	}
	activity.NewTracker(d.Recorder, d.Order).Subscribe(bus) // every customer-facing action onto the analytics spine, so the profile timeline reflects the whole journey
	d.BackInStock.Register(bus)                             // on restock, resolved customers enrol into the back_in_stock flow and guests get a direct mail
}

// Gives a new channel its transactional emails and the flows that deliver them, so receipts and account mail work without anyone running `skelet emails seed-transactional`
// Emails first - a seeded flow points at the slot email it sends, and SeedTransactionalFlows skips a trigger whose slot email isn't there yet
// Registered before the Signals gate below on purpose: transactional mail is not part of the analytics suite
// Best-effort, since the channel already exists - both seeders are idempotent, so the CLI still fixes a channel this missed
func registerChannelSeed(bus *events.Bus, tmpl *emailtemplate.Service, flows *flow.Service) {
	bus.Subscribe(channel.EventCreated, func(ctx context.Context, e events.Event) error {
		channelID := e.(*channel.Created).ChannelID
		if _, err := tmpl.SeedTransactional(ctx, channelID); err != nil {
			log.Printf("[channel] seed transactional emails for %s: %v", channelID, err)
			return nil // the flows would all skip without their slot emails
		}
		if _, err := flows.SeedTransactionalFlows(ctx, channelID); err != nil {
			log.Printf("[channel] seed transactional flows for %s: %v", channelID, err)
		}
		return nil
	})
}

// Everything reacting to an order's lifecycle - all outbox-durable, so every handler here has to tolerate a replay
func subscribeOrderReactors(d subscriberDeps) {
	bus := d.Bus
	d.Inventory.SubscribeToOrderEvents(bus) // called after registerEventSubscribers - the pre-tx reservation shares sync priority 500 with the order.placing logger there, and equal priorities break the tie by registration order
	d.Inventory.SubscribeToCartEvents(bus)
	d.Giftcard.SubscribeToOrderEvents(bus)    // mints one redeemable card per gift-card line
	d.StoreCredit.SubscribeToOrderEvents(bus) // credits redeemed amounts back when an order is cancelled
	d.Loyalty.SubscribeToOrderEvents(bus)     // earned on placement, revoked on cancel or refund
	d.Referral.SubscribeToOrderEvents(bus)    // give-get payout on the referee's first order
	d.Download.SubscribeToOrderEvents(bus)    // grants the buyer access to a purchased product's downloads
	d.Subscription.SubscribeToOrderEvents(bus)
}

// The demo loggers, the guest-order claim, and the payment→order settle bridge
func registerEventSubscribers(bus *events.Bus, webhookDispatcher *webhook.Dispatcher, orderSvc *order.Service) {
	bus.Subscribe(customer.EventRegistered, func(ctx context.Context, e events.Event) error { // async demo: log every new customer - fires post-commit, cannot abort
		evt := e.(*customer.CustomerRegistered)
		log.Printf("[events] customer.registered id=%s email=%s source=%s tags=%v",
			evt.CustomerID, evt.Email, evt.Source, evt.Tags)
		return nil
	})

	// Keyed on EMAIL VERIFIED, never registration - registering proves nothing about owning the address, so claiming there would hand an attacker every guest order placed under any email they type; the emailed link is the proof
	bus.Subscribe(customer.EventEmailVerified, func(ctx context.Context, e events.Event) error {
		evt := e.(*customer.CustomerEmailVerified)
		n, err := orderSvc.ClaimGuestOrdersByEmail(ctx, evt.CustomerID, evt.Email, evt.ChannelID)
		if err != nil {
			log.Printf("[events] guest-order claim failed for %s: %v", evt.CustomerID, err)
			return err // claim-by-email is idempotent, so the outbox replay just picks up whatever is still unclaimed
		}
		if n > 0 {
			log.Printf("[events] claimed %d guest orders for verified customer %s", n, evt.CustomerID)
		}
		return nil
	})

	bus.Subscribe(customer.EventUpdated, func(ctx context.Context, e events.Event) error { // Async demo: log every admin customer update
		evt := e.(*customer.CustomerUpdated)
		log.Printf("[events] customer.updated id=%s changed=%v source=%s tags=%v",
			evt.CustomerID, evt.ChangedFields, evt.Source, evt.Tags)
		return nil
	})

	bus.Subscribe(order.EventPlaced, func(ctx context.Context, e events.Event) error { // async demo: log every placed order - subscribers that MUST run (payment capture, 3PL dispatch) should live behind an outbox, this async mode is best-effort
		evt := e.(*order.OrderPlaced)
		customer := "guest"
		if evt.CustomerID != nil {
			customer = evt.CustomerID.String()
		}
		log.Printf("[events] order.placed id=%s number=%s customer=%s email=%s total=%d %s",
			evt.OrderID, evt.OrderNumber, customer, evt.Email, evt.TotalCents, evt.Currency)
		return nil
	})

	// Moves an order off "unpaid" - service calls AND provider webhooks both publish here, and SettlePayment guards the transitions so it stays idempotent under webhook replay and out-of-order delivery
	settle := func(ctx context.Context, orderID uuid.UUID, status, ref string) error { // returns the failure so outbox-routed payment events replay - SettlePayment's transition guards make the retry idempotent
		if orderID == uuid.Nil || status == "" {
			return nil
		}
		if _, err := orderSvc.SettlePayment(ctx, orderID, status, ref); err != nil {
			log.Printf("[events] settle payment for order %s (%s): %v", orderID, status, err)
			return err
		}
		return nil
	}
	bus.Subscribe(payment.EventAuthorized, func(ctx context.Context, e events.Event) error {
		evt := e.(*payment.PaymentAuthorized)
		return settle(ctx, evt.OrderID, order.PaymentAuthorized, evt.ProviderReference)
	})
	// Record the AMOUNT and let status derive - blind name mapping made a $10 goodwill refund mark a $500 order fully refunded, and a partial capture read as paid in full
	settleAmount := func(ctx context.Context, orderID uuid.UUID, kind string, amount int64, slug, ref string) error { // same replay contract as settle - SettleGatewayAmount dedupes per (kind, ref)
		if orderID == uuid.Nil {
			return nil
		}
		if amount <= 0 || ref == "" {
			fallback := order.PaymentPaid // No amount to reconcile against - fall back to the coarse label
			if kind == order.PaymentTxnRefund {
				fallback = order.PaymentRefunded
			}
			return settle(ctx, orderID, fallback, ref)
		}
		if _, err := orderSvc.SettleGatewayAmount(ctx, orderID, kind, amount, slug, ref); err != nil {
			log.Printf("[events] settle gateway %s for order %s: %v", kind, orderID, err)
			return err
		}
		return nil
	}
	bus.Subscribe(payment.EventCaptured, func(ctx context.Context, e events.Event) error {
		evt := e.(*payment.PaymentCaptured)
		return settleAmount(ctx, evt.OrderID, order.PaymentTxnCapture, evt.AmountCents, evt.ProviderSlug, evt.ProviderReference)
	})
	bus.Subscribe(payment.EventRefunded, func(ctx context.Context, e events.Event) error {
		evt := e.(*payment.PaymentRefunded)
		return settleAmount(ctx, evt.OrderID, order.PaymentTxnRefund, evt.AmountCents, evt.ProviderSlug, evt.ProviderReference)
	})
	bus.Subscribe(payment.EventVoided, func(ctx context.Context, e events.Event) error {
		evt := e.(*payment.PaymentVoided)
		return settle(ctx, evt.OrderID, order.PaymentVoided, evt.ProviderReference)
	})
	bus.Subscribe(payment.EventProviderWebhook, func(ctx context.Context, e events.Event) error { // the path that settles an order when the gateway confirms out-of-band (Stripe client-confirm, Braintree settlement)
		evt := e.(*payment.ProviderWebhook)
		// settle by AMOUNT when the plugin reports one (a partial capture must not flip the whole order) - absent falls back to the old coarse label
		if kind, amountAware := providerWebhookTxnKind(evt.EventName); amountAware {
			return settleAmount(ctx, evt.OrderID, kind, evt.AmountCents, evt.ProviderSlug, evt.ProviderReference)
		}
		return settle(ctx, evt.OrderID, paymentEventToOrderStatus(evt.EventName), evt.ProviderReference)
	})

	// Sync demo at priority 500 (middle tier) - abort the chain by returning an error; enrich the payload by mutating exported fields - runs pre-tx - if this returns an error, PlaceOrder returns it to the caller and no DB writes happen
	bus.SubscribeSync(order.EventPlacing, 500, func(ctx context.Context, e events.Event) error {
		evt := e.(*order.OrderPlacing)
		customer := "guest"
		if evt.CustomerID != nil {
			customer = evt.CustomerID.String()
		}
		log.Printf("[events] order.placing customer=%s email=%s subtotal=%d lines=%d",
			customer, evt.Email, evt.Subtotal, len(evt.Cart.Lines))
		return nil
	})

	// Async events get Subscribe (retried); sync events get SubscribeSync at 1000, after the demos at 500, where they can reject or mutate the payload
	for _, evt := range webhook.AllEvents() {
		switch evt.Kind {
		case webhook.KindAsync:
			bus.Subscribe(evt.Type, webhookDispatcher.Handler())
		case webhook.KindSync:
			bus.SubscribeSync(evt.Type, 1000, webhookDispatcher.SyncHandler())
		}
	}
}

// providerWebhookTxnKind picks the amount-aware settlement path for the two event names where a partial
// amount changes the outcome - everything else keeps plain name mapping
func providerWebhookTxnKind(eventName string) (kind string, amountAware bool) {
	switch eventName {
	case payment.EventCaptured:
		return order.PaymentTxnCapture, true
	case payment.EventRefunded:
		return order.PaymentTxnRefund, true
	}
	return "", false
}

func paymentEventToOrderStatus(eventName string) string { // maps the payment.* event names plugins emit to an order payment status - an empty string for unrecognized names, which the bridge treats as no-ops
	switch eventName {
	case payment.EventAuthorized:
		return order.PaymentAuthorized
	case payment.EventCaptured:
		return order.PaymentPaid
	case payment.EventRefunded:
		return order.PaymentRefunded
	case payment.EventVoided:
		return order.PaymentVoided
	}
	return ""
}
