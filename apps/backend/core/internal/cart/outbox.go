package cart

import (
	"context"
	"database/sql"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

type OutboxEnqueuer interface {
	Enqueue(context.Context, *sql.Tx, events.Event) error
}

type OutboxTypes interface {
	RegisterType(string, func() events.Event)
}

func RegisterOutboxTypes(r OutboxTypes) {
	r.RegisterType(EventOfferSent, func() events.Event { return &OfferSent{} })
	r.RegisterType(EventOfferOpened, func() events.Event { return &OfferOpened{} })
	r.RegisterType(EventOfferExpired, func() events.Event { return &OfferExpired{} })
}

func (s *Service) SetOutbox(o OutboxEnqueuer) { s.outbox = o }

// With no outbox (unit tests), publish only after the write succeeds.
func (s *Service) offerEventTransaction(ctx context.Context, write func(Repository, func(events.Event) error) error) error {
	var pending []events.Event
	run := func(tx *sql.Tx) error {
		repo := s.repo
		if tx != nil {
			repo = repo.WithTx(tx)
		}
		return write(repo, func(e events.Event) error {
			if s.outbox != nil {
				return s.outbox.Enqueue(ctx, tx, e)
			}
			pending = append(pending, e)
			return nil
		})
	}
	var err error
	if s.tx != nil {
		err = s.tx.InTransaction(ctx, run)
	} else {
		err = run(nil)
	}
	if err != nil {
		return err
	}
	if s.bus != nil {
		for _, e := range pending {
			s.bus.Publish(ctx, e)
		}
	}
	return nil
}
