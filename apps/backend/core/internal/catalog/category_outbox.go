package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
)

// OutboxTypes is the registry half of the outbox, satisfied by *outbox.Outbox
type OutboxTypes interface {
	RegisterType(eventType string, fn func() events.Event)
}

// OutboxEnqueuer writes an event through the caller's tx, satisfied by *outbox.Outbox
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, sqlTx *sql.Tx, e events.Event) error
}

func RegisterOutboxTypes(r OutboxTypes) {
	r.RegisterType(EventCategoryMoved, func() events.Event { return &CategoryMoved{} })
}

func (s *core) SetOutbox(o OutboxEnqueuer) { s.outbox = o }

// Subscribes the catalog's own async reactions - the product URL regeneration a category move owes
func (s *Service) RegisterSubscribers(bus *events.Bus) {
	bus.Subscribe(EventCategoryMoved, s.onCategoryMoved)
}

func (s *categoryService) enqueueCategoryMoved(ctx context.Context, t *sql.Tx, e *CategoryMoved) error {
	if s.outbox == nil {
		return nil
	}
	return s.outbox.Enqueue(ctx, t, e)
}

// Without an outbox the bus carries it, and with no bus either (the CLI) the regeneration runs inline so it still happens
func (s *categoryService) publishCategoryMovedUndurably(ctx context.Context, e *CategoryMoved) {
	if s.bus != nil {
		s.bus.Publish(ctx, e)
		return
	}
	if err := s.onCategoryMoved(ctx, e); err != nil {
		applog.From(ctx).Error("regenerate product urls under moved category", "category_id", e.CategoryID, "err", err)
	}
}

// Idempotent - every product is re-rendered from current data, so an outbox replay lands on the same URLs
func (s *categoryService) onCategoryMoved(ctx context.Context, e events.Event) error {
	ev, ok := e.(*CategoryMoved)
	if !ok {
		return nil
	}
	ids, err := s.repo.ListProductIDsInPrimaryCategoryDescendants(ctx, ev.CategoryID)
	if err != nil {
		return fmt.Errorf("list products under category subtree %s: %w", ev.CategoryID, err)
	}
	var errs []error
	for _, pid := range ids {
		if err := s.regenerateProductURLPath(ctx, pid, true); err != nil {
			errs = append(errs, fmt.Errorf("regenerate product %s: %w", pid, err))
			continue
		}
		p, err := s.repo.GetProductByID(ctx, pid)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		s.publishProductUpdated(ctx, p, "url")
	}
	return errors.Join(errs...)
}
