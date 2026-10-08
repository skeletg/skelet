package customer

import (
	"context"
	"database/sql"
	"log"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
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

// SetOutbox makes the registered events durable - enqueued after the write, since the delete path revokes sessions and purges the suite's identity in between
// A crash in that window leaves the roster to the 6-hourly sweep, which is what re-syncs and prunes it. Nil leaves the events on the bus
func (s *Service) SetOutbox(o OutboxEnqueuer, txm *txrunner.Manager) { s.outbox, s.tx = o, txm }

func (s *Service) publishDurable(ctx context.Context, e events.Event) {
	if s.outbox != nil && s.tx != nil {
		err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
			return s.outbox.Enqueue(ctx, sqlTx, e)
		})
		if err == nil {
			return
		}
		log.Printf("[customer] outbox enqueue %s failed, falling back to bus: %v", e.Type(), err)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, e)
	}
}
