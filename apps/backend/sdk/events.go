package sdk

import (
	"context"
	"encoding/json"
	"fmt"
)

// Post-commit reaction, handed the event pre-marshaled so the plugin decodes into its own shape without importing internal/* event types
// Errors are logged and dropped - async is fire-and-forget, and one plugin's failure can't touch other subscribers or the originating operation
type AsyncHandler func(ctx context.Context, payload json.RawMessage) error

// Pre-commit participation, handed the LIVE event value rather than a copy so exported fields can be mutated and the change flows into the emitting service
// The plugin type-asserts to the emitting domain's concrete pointer, e.g. *order.OrderPlacing - a non-nil error aborts the chain and the originating operation
type SyncHandler func(ctx context.Context, e any) error

// Handlers fan out in registration order on a background goroutine - subscribe to core domain events (internal/order/events.go, internal/customer/events.go) or anything a plugin declares via RegisterEvent
// The host skips the handler when the plugin's kill switch is off and bounds it with the plugin's hook timeout - cooperative, so a handler ignoring ctx still runs to completion
// Subscriptions made in init() replay once main.go wires the bus; after that they attach directly
//
//	p.SubscribeAsync("order.placed", func(ctx context.Context, payload json.RawMessage) error {
//	    var e struct{ OrderID uuid.UUID; TotalAmount int64 }
//	    if err := json.Unmarshal(payload, &e); err != nil {
//	        return err
//	    }
//	    return mySink.RecordOrder(ctx, e.OrderID, e.TotalAmount)
//	})
func (pl *Plugin) SubscribeAsync(eventType string, handler AsyncHandler) {
	withHost(func(h Host) error { return h.SubscribeAsync(pl.slug, eventType, handler) })
}

// PublishAsync puts the plugin's own event on the bus: async subscribers - core's, other
// plugins', and outbound webhooks when the name is RegisterEvent'd - receive the marshaled payload
// eventType MUST appear in this plugin's Manifest.Emits, or the host refuses: the boot validator
// resolves every Consumes against the Emits lists, so an undeclared publish would be invisible to
// the contract that keeps subscriber wiring honest
// Runtime, not registration - it needs a live host, so calling it before boot completes errors
func (pl *Plugin) PublishAsync(ctx context.Context, eventType string, payload any) error {
	h := hostNow()
	if h == nil {
		return fmt.Errorf("sdk: PublishAsync(%s) before the host is installed", eventType)
	}
	return h.PublishEvent(ctx, pl.slug, eventType, payload)
}

// DedupKeyFunc derives the idempotency key from the payload - an order id, a provider reference, anything stable across redeliveries of the same event
type DedupKeyFunc func(payload json.RawMessage) (string, error)

// SubscribeAsyncOnce is SubscribeAsync with at-most-once delivery per key: the outbox redelivers
// events at-least-once, and this claims a durable marker before the handler runs so a replay of
// an already-processed key is skipped - a handler ERROR releases the claim and the next replay
// retries; a crash between claim and completion loses that one key, the narrow window the
// simpler contract accepts
// Degrades to plain SubscribeAsync on a host with no dedup store wired
func (pl *Plugin) SubscribeAsyncOnce(eventType string, key DedupKeyFunc, handler AsyncHandler) {
	withHost(func(h Host) error { return h.SubscribeAsyncOnce(pl.slug, eventType, key, handler) })
}

// priority orders the chain low-to-high, ties keeping registration order - use it to sequence several plugins, e.g. validate at 100, enrich at 200, final-check at 300
// Mutation through a pointer field on the event is the deliberate seam for altering core behaviour without forking, and returning an error is how a plugin vetoes an operation outright
// A disabled plugin is skipped (nil, operation continues unmodified) rather than aborting, and sync handlers get NO timeout - they run under the caller's deadline, because cutting one mid-mutation is worse than letting it finish
//
// This is the only place plugins are expected to import internal/* - pointer mutation costs structural coupling to the emitting domain's event shape, so a rename there breaks the plugin at compile time, which is the failure mode we want:
//
//	p.SubscribeSync(cart.EventLineAdding, 100, func(ctx context.Context, e any) error {
//	    ev, ok := e.(*cart.LineAdding)
//	    if !ok {
//	        return nil // not our event shape, leave it alone
//	    }
//	    if price := staffPrice(ev.ProductSku); price > 0 {
//	        ev.UnitPrice = &price // nil would have meant "keep the input price"
//	    }
//	    return nil
//	})
func (pl *Plugin) SubscribeSync(eventType string, priority int, handler SyncHandler) {
	withHost(func(h Host) error { return h.SubscribeSync(pl.slug, eventType, priority, handler) })
}
