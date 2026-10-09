package customer

import (
	"context"
	"database/sql"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// OutboxTypes is the registry half of the outbox, satisfied by *outbox.Outbox
type OutboxTypes interface {
	RegisterType(eventType string, fn func() events.Event)
}

// OutboxEnqueuer writes an event through the caller's tx, satisfied by *outbox.Outbox - declared here so customer doesn't import outbox
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, sqlTx *sql.Tx, e events.Event) error
}

// The account events the Signals roster hangs off - one lost between commit and its handlers leaves the suite's copy wrong until the next sweep
// email_verified carries the guest-order claim, which nothing sweeps
func RegisterOutboxTypes(r OutboxTypes) {
	r.RegisterType(EventRegistered, func() events.Event { return &CustomerRegistered{} })
	r.RegisterType(EventUpdated, func() events.Event { return &CustomerUpdated{} })
	r.RegisterType(EventProfileUpdated, func() events.Event { return &CustomerProfileUpdated{} })
	r.RegisterType(EventDeleted, func() events.Event { return &CustomerDeleted{} })
	r.RegisterType(EventEmailVerified, func() events.Event { return &CustomerEmailVerified{} })
}

// SetOutbox makes the registered events durable - each is enqueued in the transaction of the write it reports, see atomically
// Nil leaves the events on the bus, published once the write commits
func (s *Service) SetOutbox(o OutboxEnqueuer) { s.outbox = o }
