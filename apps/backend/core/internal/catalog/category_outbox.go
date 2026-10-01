package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

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
	r.RegisterType(EventProductTypeURLTemplateChanged, func() events.Event { return &ProductTypeURLTemplateChanged{} })
	r.RegisterType(EventAttributeValueSlugChanged, func() events.Event { return &AttributeValueSlugChanged{} })
}

func (s *core) SetOutbox(o OutboxEnqueuer) { s.outbox = o }

// Subscribes the catalog's own async reactions - the product URL regenerations a category move, a template change or a value rename owe
func (s *Service) RegisterSubscribers(bus *events.Bus) {
	bus.Subscribe(EventCategoryMoved, s.onCategoryMoved)
	bus.Subscribe(EventProductTypeURLTemplateChanged, s.onProductTypeURLTemplateChanged)
	bus.Subscribe(EventAttributeValueSlugChanged, s.onAttributeValueSlugChanged)
}

func (s *core) publishUndurably(ctx context.Context, e events.Event, handle func(context.Context, events.Event) error) {
	if s.bus != nil {
		s.bus.Publish(ctx, e)
		return
	}
	if err := handle(ctx, e); err != nil {
		applog.From(ctx).Error("catalog async reaction", "event", e.Type(), "err", err)
	}
}

func (s *core) onProductTypeURLTemplateChanged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*ProductTypeURLTemplateChanged)
	if !ok {
		return nil
	}
	pt, err := s.repo.GetProductTypeWithAttributes(ctx, ev.ProductTypeID)
	if err != nil {
		return fmt.Errorf("load product type %s: %w", ev.ProductTypeID, err)
	}
	products, err := s.repo.ListProductsByType(ctx, ev.ProductTypeID)
	if err != nil {
		return fmt.Errorf("list products of type %s: %w", ev.ProductTypeID, err)
	}
	ids := make([]uuid.UUID, 0, len(products))
	for _, p := range products {
		if p.URLAuto && p.URLTemplateOverride == "" {
			ids = append(ids, p.ID)
		}
	}
	return s.regenerateProductURLs(ctx, ids, map[uuid.UUID]ProductType{pt.ID: pt})
}

func (s *core) onAttributeValueSlugChanged(ctx context.Context, e events.Event) error {
	ev, ok := e.(*AttributeValueSlugChanged)
	if !ok {
		return nil
	}
	return s.regenerateProductsForAttributeValue(ctx, ev.AttributeID, ev.ValueID)
}

func (s *core) regenerateProductURLs(ctx context.Context, ids []uuid.UUID, types map[uuid.UUID]ProductType) error {
	if types == nil {
		types = map[uuid.UUID]ProductType{}
	}
	var errs []error
	for _, pid := range ids {
		if err := s.regenerateProductURLPathTyped(ctx, pid, true, types); err != nil {
			errs = append(errs, fmt.Errorf("regenerate product %s: %w", pid, err))
			continue
		}
		s.publishProductUpdatedByID(ctx, pid, "url")
	}
	return errors.Join(errs...)
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
