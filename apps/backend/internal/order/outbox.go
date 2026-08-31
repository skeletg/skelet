package order

import "github.com/skeletg/skelet/apps/backend/internal/events"

// What the relay needs to re-materialize a stored row into the same pointer type a direct Publish would hand a subscriber
type OutboxTypes interface {
	RegisterType(eventType string, fn func() events.Event)
}

// Every type Enqueue may be handed has to be here, or the write fails inside the emitter's transaction - which is the
// point: an unregistered type surfaces at write time rather than as a poisoned row the relay can't decode
func RegisterOutboxTypes(r OutboxTypes) {
	r.RegisterType(EventPlaced, func() events.Event { return &OrderPlaced{} })
	r.RegisterType(EventStatusChanged, func() events.Event { return &OrderStatusChanged{} })               // carries inventory's cancel/fulfill compensation
	r.RegisterType(EventFulfillmentCreated, func() events.Event { return &OrderFulfillmentCreated{} })     // per-line stock finalization + capture-on-first-ship
	r.RegisterType(EventReturnReceived, func() events.Event { return &OrderReturnReceived{} })             // a received return's stock re-increment
	r.RegisterType(EventCancelled, func() events.Event { return &OrderCancelled{} })                       // was fire-and-forget after commit while its subscribers claimed outbox durability, so a pod killed in that window left the customer's money spent and the company's credit reserved
	r.RegisterType(EventRefunded, func() events.Event { return &OrderRefunded{} })                         // REWARD reversals only - a refund already returns the money, so restoring the gift card or store credit it consumed would pay the customer twice
	r.RegisterType(EventPaymentStatusChanged, func() events.Event { return &OrderPaymentStatusChanged{} }) // money-drift signal - was bus-only, so a crash between commit and handler lost it permanently
	r.RegisterType(EventEdited, func() events.Event { return &OrderEdited{} })                             // carries Old/NewAmountDue, the other drift signal
}
