package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/syncveto"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

const (
	EventStockLow     = "inventory.stock_low"
	EventStockOut     = "inventory.stock_out"
	EventStockChanged = "inventory.stock.changed"
	EventAllocating   = "inventory.allocating"
)

// Fires when a level crosses low_stock_threshold, from above it to at-or-below while still positive - async
type StockLow struct {
	InventoryItemID uuid.UUID
	LocationID      uuid.UUID
	ProductID       uuid.UUID
	Quantity        float64
	Threshold       float64
}

func (*StockLow) Type() string { return EventStockLow }

type StockOut struct {
	InventoryItemID uuid.UUID
	LocationID      uuid.UUID
	ProductID       uuid.UUID
}

func (*StockOut) Type() string { return EventStockOut }

// Fires AFTER every salable-quantity change commits, async - it carries the new quantity and signed delta so subscribers like ERP sync, back-in-stock and the search indexer act without re-reading, and Reason mirrors the ledger codes so they can filter Admin writes (manual_adjustment) report one cell: LocationID set, Before/After the physical level - Order-driven changes (placed/cancelled/fulfilled/returned) fold every touched row into ONE event per product, since an order hits many lines and locations: LocationID is nil and Before/After are the product's salable total across all its locations ProductID is the only field every subscriber needs, and it's always set
type StockChanged struct {
	InventoryItemID uuid.UUID
	LocationID      uuid.UUID
	ProductID       uuid.UUID
	Before          float64
	After           float64
	Delta           float64
	Reason          Reason
}

func (*StockChanged) Type() string { return EventStockChanged }

// Fires BEFORE reserving for a placing order - sync handlers can abort, or override the per-line warehouse through Routing (line id to location id) for warehouse-routing plugins
// A nil Routing uses the built-in allocation, and a routed line allocates from exactly that location, so a shortage or off-channel location fails it
type Allocating struct {
	OrderID   uuid.UUID
	ChannelID uuid.UUID
	Lines     []ReserveLine
	Routing   *map[uuid.UUID]uuid.UUID
}

func (*Allocating) Type() string                { return EventAllocating }
func (e *Allocating) EventChannelID() uuid.UUID { return e.ChannelID }

// Response {reject, reason, routing: {<orderLineId>: <locationId>}} - routing merges over earlier picks, last writer winning, and an unknown line id fails loudly
func (e *Allocating) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		syncveto.Response
		Routing map[string]string `json:"routing"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if len(resp.Routing) == 0 {
		return false, "", nil
	}
	lineIDs := make(map[uuid.UUID]struct{}, len(e.Lines))
	for _, l := range e.Lines {
		lineIDs[l.OrderLineID] = struct{}{}
	}
	if e.Routing == nil {
		e.Routing = new(map[uuid.UUID]uuid.UUID)
	}
	if *e.Routing == nil {
		*e.Routing = make(map[uuid.UUID]uuid.UUID, len(resp.Routing))
	}
	for lineStr, locStr := range resp.Routing {
		lineID, err := uuid.Parse(lineStr)
		if err != nil {
			return false, "", fmt.Errorf("routing line id %q: %w", lineStr, err)
		}
		if _, ok := lineIDs[lineID]; !ok {
			return false, "", fmt.Errorf("routing line id %s is not part of this allocation", lineID)
		}
		locID, err := uuid.Parse(locStr)
		if err != nil {
			return false, "", fmt.Errorf("routing location id %q: %w", locStr, err)
		}
		(*e.Routing)[lineID] = locID
	}
	return false, "", nil
}

// Wires inventory onto order events - OrderPlacing, sync and in the placement tx, reserves stock and aborts on shortage, while OrderStatusChanged, FulfillmentCreated and ReturnReceived compensate asynchronously
// Cart lines carry only product_sku, so the placing handler resolves SKU to product_id through ProductLookup and skips unresolvable lines as untracked
func (s *Service) SubscribeToOrderEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.SubscribeSync(order.EventPlacing, 500, s.onOrderPlacing)
	bus.Subscribe(order.EventPlaced, s.onOrderPlaced)
	bus.Subscribe(order.EventStatusChanged, s.onOrderStatusChanged)
	bus.Subscribe(order.EventFulfillmentCreated, s.onFulfillmentCreated)
	bus.Subscribe(order.EventReturnReceived, s.onReturnReceived)
}

// Wires the add-time stock guard onto the cart's pre-commit hooks, rejecting a line over salable stock for tracked non-backorderable products
// It fails OPEN on any error, so a hiccup never blocks shopping, and defers to placement's reservation as authoritative - LineAdding carries the incremental quantity, since the cart sums by SKU, so it catches the common case; LineUpdating carries the absolute new quantity, so that path is exact
func (s *Service) SubscribeToCartEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.SubscribeSync(cart.EventLineAdding, 500, s.onCartLineAdding)
	bus.SubscribeSync(cart.EventBundleAdding, 500, s.onCartBundleAdding)
	bus.SubscribeSync(cart.EventLineUpdating, 500, s.onCartLineUpdating)
}

func (s *Service) onCartLineAdding(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineAdding)
	if !ok {
		return nil
	}
	return s.guardCartLine(ctx, ev.ChannelID, ev.ProductSku, qtyOrOne(ev.Quantity), ev.StockParts)
}

func (s *Service) onCartBundleAdding(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.BundleAdding)
	if !ok {
		return nil
	}
	// The bundle SKU holds no stock, so its components are what can run out - placement still reserves them for real
	return s.guardCartLine(ctx, ev.ChannelID, ev.ProductSku, qtyOrOne(ev.Quantity), ev.StockParts)
}

func (s *Service) onCartLineUpdating(ctx context.Context, e events.Event) error {
	ev, ok := e.(*cart.LineUpdating)
	if !ok {
		return nil
	}
	return s.guardCartLine(ctx, ev.ChannelID, ev.ProductSku, qtyOrOne(ev.Quantity), ev.StockParts)
}

// The line's own SKU plus every linked option source it takes per unit
func (s *Service) guardCartLine(ctx context.Context, channelID uuid.UUID, sku string, qty float64, parts []cart.CartLineStockPart) error {
	if err := s.guardCartStock(ctx, channelID, sku, qty); err != nil {
		return err
	}
	if channelID == uuid.Nil || qty <= 0 || !s.ledgerEnabled(ctx) {
		return nil
	}
	need := make(map[uuid.UUID]float64, len(parts))
	for _, p := range parts {
		need[p.ProductID] += p.Quantity * qty
	}
	for productID, q := range need {
		if err := s.guardProductStock(ctx, channelID, productID, q); err != nil {
			return err
		}
	}
	return nil
}

func qtyOrOne(q *float64) float64 {
	if q == nil {
		return 1
	}
	return *q
}

// Whether the SKU can be added at qty without outrunning salable stock, fail-open on every ambiguity
// Order placement asks before granting a promotion free gift: a gift is a bonus, so one that can't be stocked has to be dropped rather than fail the reservation and take the paid order down with it
func (s *Service) CanFulfillSKU(ctx context.Context, channelID uuid.UUID, sku string, qty float64) bool {
	return s.guardCartStock(ctx, channelID, sku, qty) == nil
}

// Rejects a cart mutation over salable stock - fail-open on every ambiguity, so only a definitive "want N, have fewer" returns an error
func (s *Service) guardCartStock(ctx context.Context, channelID uuid.UUID, sku string, qty float64) error {
	if sku == "" || qty <= 0 || channelID == uuid.Nil {
		return nil
	}
	if !s.ledgerEnabled(ctx) || s.products == nil {
		return nil // inventory not authoritative here (cutover kill switch), or no catalog wired
	}
	productID, err := s.products.GetProductIDBySKU(ctx, sku)
	if err != nil {
		return nil // not a catalog product we can resolve → don't gate
	}
	return s.guardProductStock(ctx, channelID, productID, qty)
}

func (s *Service) guardProductStock(ctx context.Context, channelID, productID uuid.UUID, qty float64) error {
	item, err := s.repo.GetItemByProductID(ctx, productID)
	if err != nil {
		return nil // no inventory item (always in stock) or a lookup blip → fail open
	}
	if !item.Tracked || item.Backorderable {
		return nil // untracked or backorderable → always addable
	}
	salable, err := s.Salable(ctx, productID, channelID)
	if err != nil || salable < 0 {
		return nil // error → fail open; -1 sentinel → always in stock
	}
	if qty > salable {
		return apperr.New("CART_INSUFFICIENT_STOCK", apperr.ClassConflict,
			fmt.Sprintf("only %g left in stock", salable))
	}
	return nil
}

func (s *Service) onOrderPlacing(ctx context.Context, e events.Event) error {
	placing, ok := e.(*order.OrderPlacing)
	if !ok {
		return nil
	}
	// Both modes gate placement - flat decrements item quantities, multi-location writes ledger holds
	// The ledger toggle alone is the "another platform owns inventory" kill switch, checked inside ReserveTx too
	if !s.ledgerEnabled(ctx) {
		return nil
	}
	if s.products == nil {
		return nil // no product lookup → don't gate placement on inventory
	}
	lines, err := s.linesFromCart(ctx, placing.Cart)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	// The order id is pre-allocated so reservation rows link before the order row exists - join the placement tx when provided, so they commit or roll back atomically
	if placing.Tx != nil {
		if _, err := s.ReserveTx(ctx, placing.Tx, placing.ChannelID, placing.OrderID, lines); err != nil {
			return err
		}
	} else if _, err := s.Reserve(ctx, placing.ChannelID, placing.OrderID, lines); err != nil {
		return err
	}
	placing.Tags = append(placing.Tags, "stock-reserved")
	return nil
}

// Resolves each cart line to one or more ReserveLines - simple lines emit one, and bundle lines decompose into one per component, since the wrapper owns no stock
// Component quantities are pre-multiplied and every component links to the parent OrderLineID for clean cancellation - Unresolvable lines are skipped, erring toward letting the order through rather than blocking on bookkeeping gaps
func (s *Service) linesFromCart(ctx context.Context, c cart.Cart) ([]ReserveLine, error) {
	out := make([]ReserveLine, 0, len(c.Lines))
	for _, line := range c.Lines {
		if len(line.Components) > 0 {
			for _, comp := range line.Components {
				out = append(out, ReserveLine{
					ProductID:   comp.ProductID,
					OrderLineID: line.ID,
					Quantity:    comp.Quantity,
				})
			}
			continue
		}
		if len(line.StockParts) > 0 {
			for _, part := range line.StockParts {
				out = append(out, ReserveLine{
					ProductID:   part.ProductID,
					OrderLineID: line.ID,
					Quantity:    part.Quantity * line.Quantity,
				})
			}
			if !cart.OwnsStock(line.StockParts) {
				continue
			}
		}
		if line.ProductSku == "" || line.IsCustom { // a custom offer line has no product to hold stock for
			continue
		}
		productID, err := s.products.GetProductIDBySKU(ctx, line.ProductSku)
		if err != nil {
			// Don't fail placement on a missing SKU - the cart asserted it at add-time
			log.Printf("[inventory] resolve sku %q: %v", line.ProductSku, err)
			continue
		}
		// Kit lines expand into their components, which own the stock, with quantities multiplied by the line quantity - non-kit products reserve as a single unit
		// A lookup FAILURE is not the same as "not a kit": we can't tell whether this line owns stock directly or through components, and falling through would reserve the wrapper, which owns none - the order would commit tagged stock-reserved while nothing was actually held, and the kit would keep overselling
		comps, kerr := s.products.KitComponents(ctx, productID)
		if kerr != nil {
			return nil, fmt.Errorf("kit components for %q: %w", line.ProductSku, kerr)
		}
		if len(comps) > 0 {
			for _, comp := range comps {
				out = append(out, ReserveLine{
					ProductID:   comp.ProductID,
					OrderLineID: line.ID,
					Quantity:    comp.Quantity * line.Quantity,
				})
			}
			continue
		}
		out = append(out, ReserveLine{
			ProductID:   productID,
			OrderLineID: line.ID,
			Quantity:    line.Quantity,
		})
	}
	return out, nil
}

// Announces the placement's stock movement - reservation happens inside the order's own transaction, so this is the first moment inventory knows it committed, and publishing from ReserveTx would emit a phantom event for a rolled-back order
func (s *Service) onOrderPlaced(ctx context.Context, e events.Event) error {
	evt, ok := e.(*order.OrderPlaced)
	if !ok || !s.ledgerEnabled(ctx) {
		return nil
	}
	if s.flatModeEnabled(ctx) {
		s.publishFlatPlacementStock(ctx, evt.OrderID)
		return nil
	}
	rows, err := s.repo.ListReservationsForOrder(ctx, evt.OrderID)
	if err != nil {
		log.Printf("[inventory] stock event reservations for order %s: %v", evt.OrderID, err)
		return err
	}
	// Placement holds only - a fulfillment or cancel racing this must not fold its compensations into the placement delta
	held := make(map[uuid.UUID]float64, len(rows))
	for _, row := range rows {
		if row.Reason != ReasonOrderPlaced {
			continue
		}
		held[row.InventoryItemID] += row.QuantityDelta
	}
	s.publishStockChanged(ctx, held, ReasonOrderPlaced)
	return nil
}

// Flat mode writes no ledger, so the placement's movement is rebuilt from the order's lines, which already come expanded into the components that own the stock
func (s *Service) publishFlatPlacementStock(ctx context.Context, orderID uuid.UUID) {
	if s.orders == nil {
		return
	}
	lines, err := s.orders.GetOrderLines(ctx, orderID)
	if err != nil {
		log.Printf("[inventory] stock event order lines for %s: %v", orderID, err)
		return
	}
	byProduct := make(map[uuid.UUID]float64, len(lines))
	for _, l := range lines {
		byProduct[l.ProductID] -= l.Quantity
	}
	s.publishStockChangedForProducts(ctx, byProduct, ReasonOrderPlaced)
}

func (s *Service) onOrderStatusChanged(ctx context.Context, e events.Event) error {
	change, ok := e.(*order.OrderStatusChanged)
	if !ok {
		return nil
	}
	// Cancellation releases the still-held reservations, since shipped lines already netted to zero through FulfillLines, so only the unshipped remainder restocks - fulfillment runs per-shipment via onFulfillmentCreated, not here
	// Flat mode restores through the same entry point (ReleaseForOrder routes internally); its restore is line-set based, relying on the order service rejecting cancellation after fulfillment
	var op func(context.Context) error
	switch change.ToStatus {
	case "cancelled":
		op = func(ctx context.Context) error {
			return s.ReleaseForOrder(ctx, change.OrderID, ReasonOrderCancelled)
		}
	default:
		return nil
	}
	// Release and fulfill are idempotent, since ledger chains net to zero, so they're retry-safe here and replay-safe under the outbox
	// This is the last line of defense for inventory accuracy, so it retries rather than log-and-drop a transient failure
	if err := retryWithBackoff(ctx, 5, 100*time.Millisecond, op); err != nil {
		log.Printf("[inventory] %s compensation for order %s FAILED after retries (stock may be inconsistent): %v",
			change.ToStatus, change.OrderID, err)
		return err
	}
	return nil
}

// Decrements reserved stock for exactly the shipped lines - idempotent, since FulfillLines caps each line at its still-held quantity, so a redelivered event consumes nothing once the hold is gone
func (s *Service) onFulfillmentCreated(ctx context.Context, e events.Event) error {
	evt, ok := e.(*order.OrderFulfillmentCreated)
	if !ok {
		return nil
	}
	// Flat mode already decremented at placement - shipping moves nothing further
	if s.flatModeEnabled(ctx) {
		return nil
	}
	lines := make([]FulfillLineQty, len(evt.Lines))
	for i, l := range evt.Lines {
		lines[i] = FulfillLineQty{OrderLineID: l.OrderLineID, Quantity: l.Quantity}
	}
	op := func(ctx context.Context) error { return s.FulfillLines(ctx, evt.OrderID, lines) }
	if err := retryWithBackoff(ctx, 5, 100*time.Millisecond, op); err != nil {
		log.Printf("[inventory] fulfillment finalize for order %s FAILED after retries (stock may be inconsistent): %v",
			evt.OrderID, err)
		return err
	}
	return nil
}

// Adds returned quantities back to sellable stock - idempotent, since RestockReturn caps each line at the quantity that originally shipped from each cell
func (s *Service) onReturnReceived(ctx context.Context, e events.Event) error {
	evt, ok := e.(*order.OrderReturnReceived)
	if !ok {
		return nil
	}
	// Flat mode has no shipped-from bookkeeping to restock against - returned stock re-enters via the ERP push or a manual adjustment
	if s.flatModeEnabled(ctx) {
		return nil
	}
	lines := make([]RestockLineQty, len(evt.Lines))
	for i, l := range evt.Lines {
		lines[i] = RestockLineQty{OrderLineID: l.OrderLineID, Quantity: l.Quantity}
	}
	op := func(ctx context.Context) error { return s.RestockReturn(ctx, evt.OrderID, evt.ReturnID, lines) }
	if err := retryWithBackoff(ctx, 5, 100*time.Millisecond, op); err != nil {
		log.Printf("[inventory] return restock for order %s FAILED after retries (stock may be inconsistent): %v",
			evt.OrderID, err)
		return err
	}
	return nil
}

// Runs op up to attempts times with a doubling delay, returning the last error on total failure - aborts on ctx cancel so a parked retry can't outlive shutdown
func retryWithBackoff(ctx context.Context, attempts int, initial time.Duration, op func(context.Context) error) error {
	delay := initial
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = op(ctx); err == nil {
			return nil
		}
		if attempt == attempts {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("aborted after %d attempt(s): %w", attempt, err)
		case <-time.After(delay):
		}
		delay *= 2
	}
	return err
}

// Reservation rows carry order_id from the moment they're written, since the order service pre-allocates the UUID before OrderPlacing - so ReleaseForOrder and MarkFulfilled look them up directly, with no post-tx backfill
