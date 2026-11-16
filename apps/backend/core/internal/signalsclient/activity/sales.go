package activity

import (
	"context"
	"maps"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
)

// A product's net sales on the spine is the sum over three events, each booked on the day it happens so last month
// never moves: the lines as placed, what an edit moved on each, and what a refund took back

// saleLine is one of those events: what every one of them carries, its dedupe key, and what only its kind adds
type saleLine struct {
	key    string
	lineID uuid.UUID
	sku    string
	qty    float64
	net    int64
	extra  map[string]any
}

// emitLines books one kind of sales line for an order, with the net in the base currency where a rate is known
func (t *Tracker) emitLines(ctx context.Context, name string, ref orderRef, o order.Order, lines []saleLine) error {
	if len(lines) == 0 {
		return nil
	}
	toBase := t.base.From(ctx, o.CurrencyCode)
	for _, l := range lines {
		props := map[string]any{
			"order_id": o.ID.String(), "line_id": l.lineID.String(), "sku": l.sku, "qty": l.qty, "net_cents": l.net, "currency": o.CurrencyCode,
		}
		if b, ok := toBase(l.net); ok {
			props["net_base_cents"] = b
		}
		maps.Copy(props, l.extra)
		if err := t.emitAs(ctx, sourceCommerce, name, ref.channel, ref.who, l.key, props); err != nil {
			return err
		}
	}
	return nil
}

// recordPlacedLines is one event per line, so a product's orders, units and net sales can be read - and a board
// narrowed to its sku
func (t *Tracker) recordPlacedLines(ctx context.Context, ref orderRef, o order.Order) error {
	net := order.LineNet(o)
	lines := make([]saleLine, 0, len(o.Lines))
	for _, l := range o.Lines {
		lines = append(lines, saleLine{
			key: "act:order_line:" + l.ID.String(), lineID: l.ID, sku: l.ProductSku, qty: l.Quantity, net: net[l.ID],
			extra: map[string]any{"gross_cents": money.LineTotal(l.UnitPrice, l.Quantity), "is_gift": l.IsGift},
		})
	}
	return t.emitLines(ctx, signalsapi.EventOrderLine, ref, o, lines)
}

// recordLineEdits books what an edit moved on each line as the difference in net sales, on the day of the edit
func (t *Tracker) recordLineEdits(ctx context.Context, ref orderRef, after order.Order, before []order.EditedLine) error {
	adj := order.LineAdjustments(before, after)
	lines := make([]saleLine, 0, len(adj))
	for _, a := range adj {
		lines = append(lines, saleLine{
			key: "act:order_line_adj:" + a.LineID.String() + ":" + itoa(secBucket(ctx)), lineID: a.LineID, sku: a.ProductSku, qty: a.Quantity, net: a.NetCents,
			extra: map[string]any{"added": a.Added, "removed": a.Removed},
		})
	}
	return t.emitLines(ctx, signalsapi.EventOrderLineAdjusted, ref, after, lines)
}

// recordRefundLines books each line a credit memo covered with negative amounts. A memo with no lines (a shipping or
// goodwill refund) belongs to no product and books nothing
func (t *Tracker) recordRefundLines(ctx context.Context, ref orderRef, o order.Order, memo order.CreditMemo) error {
	skus := make(map[uuid.UUID]string, len(o.Lines))
	for _, l := range o.Lines {
		skus[l.ID] = l.ProductSku
	}
	lines := make([]saleLine, 0, len(memo.Lines))
	for _, ml := range memo.Lines {
		lines = append(lines, saleLine{
			key: "act:refund_line:" + ml.ID.String(), lineID: ml.OrderLineID, sku: skus[ml.OrderLineID], qty: -ml.Quantity,
			net:   -order.LineNetFor(o, ml.OrderLineID, ml.Quantity),
			extra: map[string]any{"refund_cents": ml.TotalCents},
		})
	}
	return t.emitLines(ctx, signalsapi.EventRefundLine, ref, o, lines)
}

// BackfillRefund replays a past credit memo's refund lines at the time it was issued
func (t *Tracker) BackfillRefund(ctx context.Context, o order.Order, memo order.CreditMemo) error {
	return t.recordRefundLines(events.WithEmittedAt(ctx, memo.CreatedAt), refOf(o), o, memo)
}
