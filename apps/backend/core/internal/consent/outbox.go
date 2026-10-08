package consent

import (
	"context"
	"database/sql"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// OutboxTypes is the registry half of the outbox, satisfied by *outbox.Outbox
type OutboxTypes interface {
	RegisterType(eventType string, fn func() events.Event)
}

// OutboxEnqueuer writes an event through the caller's tx, satisfied by *outbox.Outbox
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, sqlTx *sql.Tx, e events.Event) error
}

// A consent change must not be lost - an unsubscribe that never reaches the suite is a campaign sent to someone who said no
func RegisterOutboxTypes(r OutboxTypes) {
	r.RegisterType(EventChanged, func() events.Event { return &ConsentChanged{} })
}

// SetOutbox makes the change event durable, relayed at least once
func (s *Service) SetOutbox(o OutboxEnqueuer, txm *txrunner.Manager) { s.outbox, s.tx = o, txm }

// The row, its history entry and its event commit together where the outbox is wired - an opt-out that reached the database and not the suite is a campaign sent to someone who said no
func (s *Service) upsertAndPublish(ctx context.Context, c Change) (StoredConsent, error) {
	if s.outbox == nil || s.tx == nil {
		stored, err := s.repo.Upsert(ctx, c)
		if err == nil && s.bus != nil {
			s.bus.Publish(ctx, &ConsentChanged{CustomerID: c.CustomerID, Kind: c.Kind, OptedIn: stored.OptedIn, Source: stored.Source})
		}
		return stored, err
	}
	var stored StoredConsent
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		var err error
		if stored, err = s.repo.UpsertTx(ctx, sqlTx, c); err != nil {
			return err
		}
		return s.outbox.Enqueue(ctx, sqlTx, &ConsentChanged{CustomerID: c.CustomerID, Kind: c.Kind, OptedIn: stored.OptedIn, Source: stored.Source})
	})
	return stored, err
}
