package order

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	shippingpkg "github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// rateItemsFromLines summarizes order lines for a carrier rate request - what the carrier prices, and their value the shipping subtotal - a custom offer line never ships, so placement left it out of both
func rateItemsFromLines(lines []OrderLine) ([]shippingpkg.RateItem, int64) {
	out := make([]shippingpkg.RateItem, 0, len(lines))
	var subtotal int64
	for _, l := range lines {
		if l.IsCustom {
			continue
		}
		out = append(out, shippingpkg.RateItem{SKU: l.ProductSku, Quantity: l.Quantity, UnitPriceCents: l.UnitPrice})
		subtotal += money.LineTotal(l.UnitPrice, l.Quantity)
	}
	return out, subtotal
}

// editableStatuses are the statuses an edit may mutate - strictly pre-fulfillment
// (everything reserved, nothing shipped) - Once (partially) fulfilled, returns/refunds are the path
var editableStatuses = map[string]bool{
	OrderStatusDraft:      true,
	OrderStatusPending:    true,
	OrderStatusConfirmed:  true,
	OrderStatusProcessing: true,
}

// EditLineQty changes an existing line's quantity
type EditLineQty struct {
	OrderLineID uuid.UUID
	Quantity    float64
}

// EditAddLine adds a new simple-product line, priced server-side
type EditAddLine struct {
	ProductSku string
	Quantity   float64
}

// EditOrderInput is a pre-fulfillment edit - quantity updates, removals, and added simple lines
type EditOrderInput struct {
	OrderID    uuid.UUID
	UpdateQty  []EditLineQty
	RemoveLine []uuid.UUID
	AddLine    []EditAddLine
}

// EditOrder applies a pre-fulfillment edit - mutates lines, holds the discount constant (clamped to the new subtotal, the promotion engine is NOT re-run), recomputes tax, re-quotes shipping (else holds it), persists the amounts, and re-reserves inventory
// all in one tx so a shortage rolls it back - Payment isn't re-settled; the amount-due drift is published on OrderEdited for capture-time reconciliation
func (s *Service) EditOrder(ctx context.Context, in EditOrderInput) (Order, error) {
	o, err := s.repo.GetOrderByID(ctx, in.OrderID)
	if err != nil {
		return Order{}, err
	}
	if !editableStatuses[o.Status] {
		return Order{}, apperr.New("ORDER_NOT_EDITABLE", apperr.ClassConflict,
			fmt.Sprintf("an order in status %q cannot be edited", o.Status))
	}
	if len(in.UpdateQty) == 0 && len(in.RemoveLine) == 0 && len(in.AddLine) == 0 {
		return Order{}, apperr.New("ORDER_EDIT_EMPTY", apperr.ClassValidation, "an edit must change at least one line")
	}
	if s.channels == nil {
		return Order{}, apperr.New("ORDER_EDIT_UNAVAILABLE", apperr.ClassUnavailable, "order editing is not configured")
	}

	byID := make(map[uuid.UUID]OrderLine, len(o.Lines))
	for _, l := range o.Lines {
		byID[l.ID] = l
	}

	removed := make(map[uuid.UUID]bool, len(in.RemoveLine))
	for _, id := range in.RemoveLine {
		if _, ok := byID[id]; !ok {
			return Order{}, apperr.New("ORDER_EDIT_UNKNOWN_LINE", apperr.ClassValidation, "removed line does not belong to this order")
		}
		removed[id] = true
	}

	qtyByID := make(map[uuid.UUID]float64, len(in.UpdateQty))
	for _, u := range in.UpdateQty {
		l, ok := byID[u.OrderLineID]
		if !ok {
			return Order{}, apperr.New("ORDER_EDIT_UNKNOWN_LINE", apperr.ClassValidation, "edited line does not belong to this order")
		}
		if removed[u.OrderLineID] {
			return Order{}, apperr.New("ORDER_EDIT_CONFLICT", apperr.ClassValidation, "a line cannot be both updated and removed")
		}
		if u.Quantity <= 0 {
			return Order{}, apperr.New("ORDER_EDIT_INVALID_QTY", apperr.ClassValidation, "edited quantity must be positive (remove the line to delete it)")
		}
		if len(l.Components) > 0 {
			return Order{}, apperr.New("ORDER_EDIT_BUNDLE_QTY", apperr.ClassConflict, "bundle line quantities can't be edited; remove and re-add instead")
		}
		qtyByID[u.OrderLineID] = u.Quantity
	}

	// Resolve added-line pricing server-side, before opening the transaction
	type resolvedAdd struct {
		sku, name string
		unit      int64
		qty       float64
		kit       []cart.CartLineStockPart
	}
	adds := make([]resolvedAdd, 0, len(in.AddLine))
	for _, a := range in.AddLine {
		if a.Quantity <= 0 {
			return Order{}, apperr.New("ORDER_EDIT_INVALID_QTY", apperr.ClassValidation, "added quantity must be positive")
		}
		if s.pricer == nil {
			return Order{}, apperr.New("ORDER_EDIT_UNAVAILABLE", apperr.ClassUnavailable, "adding lines is not configured")
		}
		p, err := s.pricer.ResolveSimplePricing(ctx, o.ChannelID, a.ProductSku)
		if err != nil {
			return Order{}, fmt.Errorf("resolve price for %q: %w", a.ProductSku, err)
		}
		if err := s.rejectRequiredOptionSKUs(ctx, o.ChannelID, a.ProductSku); err != nil {
			return Order{}, err
		}
		adds = append(adds, resolvedAdd{sku: a.ProductSku, name: p.ProductName, unit: p.UnitPriceCents, qty: a.Quantity})
	}
	if s.kits != nil {
		for i := range adds {
			if adds[i].kit, err = s.kits.KitComponentsBySKU(ctx, adds[i].sku); err != nil {
				return Order{}, fmt.Errorf("kit components for %q: %w", adds[i].sku, err)
			}
		}
	}

	store, err := s.channels.LoadChannel(ctx, o.ChannelID)
	if err != nil {
		return Order{}, fmt.Errorf("load channel: %w", err)
	}
	var cust customer.Customer
	if o.CustomerID != nil {
		if loaded, lerr := s.customerSvc.GetByIDWithAddresses(ctx, *o.CustomerID); lerr == nil {
			cust = loaded
		}
	}

	// Build the post-edit active line set in memory, retaining bundle Components (their qty is fixed)
	active := make([]OrderLine, 0, len(o.Lines)+len(adds))
	for _, l := range o.Lines {
		if removed[l.ID] {
			continue
		}
		if q, ok := qtyByID[l.ID]; ok {
			l.Quantity = q
			l.TotalPrice = money.LineTotal(l.UnitPrice, q)
		}
		active = append(active, l)
	}

	if len(active)+len(adds) == 0 {
		return Order{}, apperr.New("ORDER_EDIT_EMPTY_RESULT", apperr.ClassValidation, "an edit can't remove every line; cancel the order instead")
	}
	items, newSubtotal := rateItemsFromLines(active)
	for _, a := range adds {
		items = append(items, shippingpkg.RateItem{SKU: a.sku, Quantity: a.qty, UnitPriceCents: a.unit})
		newSubtotal += money.LineTotal(a.unit, a.qty)
	}
	quoted, requoted, err := s.requoteShipping(ctx, o, o.ShippingAddress, items, newSubtotal, store.CurrencyCode)
	if err != nil {
		return Order{}, err
	}

	oldAmountDue := o.AmountDue()
	var updated Order
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		// Re-check under the order lock - the plan above comes from an unlocked read, so a shipment, a cancel or another edit committing meanwhile was edited over: a shipped line dropped, stock re-held on a cancelled order, totals from a line set that no longer existed
		if err := lockForEdit(ctx, r, o); err != nil {
			return err
		}
		for id := range removed {
			if _, err := r.SoftDeleteOrderLine(ctx, id); err != nil {
				return err
			}
		}
		for id, q := range qtyByID {
			l := byID[id]
			if _, err := r.UpdateOrderLineQuantity(ctx, id, q, money.LineTotal(l.UnitPrice, q)); err != nil {
				return err
			}
		}
		var added []OrderLine
		for _, a := range adds {
			line, err := r.AddOrderLine(ctx, OrderLine{
				OrderID:     o.ID,
				ProductName: a.name,
				ProductSku:  a.sku,
				UnitPrice:   a.unit,
				Quantity:    a.qty,
				TotalPrice:  money.LineTotal(a.unit, a.qty),
			})
			if err != nil {
				return err
			}
			line.StockParts = orderStockParts(a.kit)
			if err := r.AddOrderLineStockParts(ctx, line.ID, line.StockParts); err != nil {
				return err
			}
			active = append(active, line)
			added = append(added, line)
		}
		if err := editShipmentLines(ctx, r, o.ID, removed, qtyByID, added); err != nil {
			return err
		}

		// Recompute money from the new line set
		var subtotal int64
		for _, l := range active {
			subtotal += money.LineTotal(l.UnitPrice, l.Quantity)
		}
		discount := min(o.DiscountAmount, subtotal)
		synthetic := syntheticCart(o, active)

		shipping := o.ShippingAmount
		if requoted {
			shipping = quoted
			if err := r.SetShipmentAmount(ctx, o.ID, shipping); err != nil {
				return err
			}
		}

		tax, shippingTax := o.TaxAmount, o.ShippingTaxAmount
		var taxLines []TaxLine
		if s.tax != nil {
			cc := &checkoutContext{customer: cust, shipping: o.ShippingAddress, store: &store, cart: synthetic, subtotal: subtotal}
			tax, err = s.computeLineTax(ctx, cc, discount, shipping)
			if err != nil {
				return err
			}
			taxLines, shippingTax = cc.taxLines, cc.shippingTax
		}

		total := computeOrderTotal(subtotal, discount, tax, shipping, store.TaxInclusivePricing, nil)
		// An edit that shrinks the order below what the non-gateway tenders already covered would leave AmountDue() negative - the resolver floors it at 0, so the overpayment was invisible on every surface. Record the drift on the timeline; the operator refunds the difference, since a gift card or store credit can't be un-spent silently.
		if tendered := o.GiftCardAmount + o.StoreCreditAmount; tendered > total {
			if _, err := r.AddOrderEvent(ctx, OrderEvent{
				OrderID: o.ID,
				Type:    "payment_overtendered",
				Message: fmt.Sprintf("Edit dropped the total to %d cents, below the %d already covered by gift card and store credit - refund the %d cents difference.", total, tendered, tendered-total),
			}); err != nil {
				return fmt.Errorf("add over-tender event: %w", err)
			}
		}
		updated, err = r.UpdateOrderAmounts(ctx, o.ID, OrderAmounts{Subtotal: subtotal, Shipping: shipping, Discount: discount, Tax: tax, ShippingTax: shippingTax, Total: total})
		if err != nil {
			return err
		}
		if err := replaceTaxLines(ctx, r, o.ID, taxLines); err != nil {
			return err
		}

		// Re-reserve for the new line set - a shortage aborts - Drafts hold no stock
		// until placed, so editing a draft only re-prices and must not reserve
		if s.inventoryEditor != nil && o.Status != OrderStatusDraft {
			if err := s.inventoryEditor.ReReserveForEditTx(ctx, sqlTx, o.ChannelID, o.ID, synthetic); err != nil {
				return err
			}
		}

		if _, err := r.AddOrderEvent(ctx, OrderEvent{
			OrderID: o.ID,
			Type:    "order_edited",
			Message: fmt.Sprintf("Order edited: total %d → %d cents.", o.TotalAmount, total),
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		return nil
	})
	if err != nil {
		return Order{}, err
	}

	// Payment-drift signal. amountDue↓ reconciles at capture (the over-capture guard
	// caps it), amountDue↑ above the authorization needs a re-auth, surfaced here
	newAmountDue := updated.AmountDue()
	if s.bus != nil {
		s.publishDurable(ctx, &OrderEdited{OrderID: o.ID, ChannelID: o.ChannelID, OldAmountDue: oldAmountDue, NewAmountDue: newAmountDue})
	}
	return s.repo.GetOrderByID(ctx, o.ID)
}

// A recompute invalidates the whole breakdown, so it is replaced rather than merged
func replaceTaxLines(ctx context.Context, repo Repository, orderID uuid.UUID, lines []TaxLine) error {
	if err := repo.DeleteTaxLines(ctx, orderID); err != nil {
		return err
	}
	for _, tl := range lines {
		if err := repo.InsertTaxLine(ctx, orderID, tl); err != nil {
			return err
		}
	}
	return nil
}

// syntheticCart projects an order + its active lines into a cart.Cart so the edit reuses the placement tax + reservation logic without
// duplicating bundle/kit decomposition - Each line carries the order line id so reservations link back
func syntheticCart(o Order, lines []OrderLine) cart.Cart {
	cl := make([]cart.CartLine, len(lines))
	for i, l := range lines {
		comps := make([]cart.CartLineComponent, len(l.Components))
		for j, c := range l.Components {
			comps[j] = cart.CartLineComponent{
				ProductID:   c.ProductID,
				ProductSku:  c.ProductSku,
				ProductName: c.ProductName,
				UnitPrice:   c.UnitPrice,
				Quantity:    c.Quantity,
			}
		}
		parts := make([]cart.CartLineStockPart, len(l.StockParts))
		for j, p := range l.StockParts {
			parts[j] = cart.CartLineStockPart{ProductID: p.ProductID, Quantity: p.Quantity, Source: p.Source}
		}
		cl[i] = cart.CartLine{
			ID:          l.ID,
			ProductSku:  l.ProductSku,
			ProductName: l.ProductName,
			UnitPrice:   l.UnitPrice,
			Quantity:    l.Quantity,
			Components:  comps,
			StockParts:  parts,
			IsCustom:    l.IsCustom,
			TaxClass:    l.TaxClass,
		}
	}
	return cart.Cart{ChannelID: o.ChannelID, CustomerID: o.CustomerID, Lines: cl}
}

// EditOrderAddressInput updates a pre-fulfillment order's shipping and/or billing snapshot, nil = leave unchanged
type EditOrderAddressInput struct {
	OrderID  uuid.UUID
	Shipping *OrderAddress
	Billing  *OrderAddress
}

func validEditAddress(a OrderAddress) error {
	if a.FirstName == "" || a.LastName == "" || a.Street1 == "" || a.City == "" || a.PostalCode == "" || len(a.CountryCode) != 2 {
		return apperr.New("ORDER_EDIT_INVALID_ADDRESS", apperr.ClassValidation,
			"address needs first/last name, street, city, postal code and a 2-letter country")
	}
	return nil
}

// EditOrderAddress rewrites the address snapshot pre-fulfillment - A shipping change can move the tax region + carrier rate, so tax is recomputed and shipping re-quoted (fail-soft, like EditOrder), with drift on OrderEdited - Once anything ships the snapshot is frozen - re-routing is a carrier op, not an edit
func (s *Service) EditOrderAddress(ctx context.Context, in EditOrderAddressInput) (Order, error) {
	o, err := s.repo.GetOrderByID(ctx, in.OrderID)
	if err != nil {
		return Order{}, err
	}
	if !editableStatuses[o.Status] {
		return Order{}, apperr.New("ORDER_NOT_EDITABLE", apperr.ClassConflict,
			fmt.Sprintf("an order in status %q cannot be edited", o.Status))
	}
	if in.Shipping == nil && in.Billing == nil {
		return Order{}, apperr.New("ORDER_EDIT_EMPTY", apperr.ClassValidation, "provide a shipping and/or billing address")
	}
	rules := s.phoneRules(ctx, o.ChannelID)
	billing, shipping := o.BillingAddress, o.ShippingAddress
	if in.Billing != nil {
		if err := validEditAddress(*in.Billing); err != nil {
			return Order{}, err
		}
		if billing, err = normalizeOrderAddress(rules, *in.Billing, "billing"); err != nil {
			return Order{}, err
		}
	}
	if in.Shipping != nil {
		if err := validEditAddress(*in.Shipping); err != nil {
			return Order{}, err
		}
		if shipping, err = normalizeOrderAddress(rules, *in.Shipping, "shipping"); err != nil {
			return Order{}, err
		}
	}

	store, err := s.channels.LoadChannel(ctx, o.ChannelID)
	if err != nil {
		return Order{}, fmt.Errorf("load channel: %w", err)
	}
	var cust customer.Customer
	if o.CustomerID != nil {
		if loaded, lerr := s.customerSvc.GetByIDWithAddresses(ctx, *o.CustomerID); lerr == nil {
			cust = loaded
		}
	}

	// GetOrderByID already filters soft-removed lines
	var subtotal int64
	for _, l := range o.Lines {
		subtotal += money.LineTotal(l.UnitPrice, l.Quantity)
	}
	var (
		quoted   int64
		requoted bool
	)
	if in.Shipping != nil {
		items, shipSubtotal := rateItemsFromLines(o.Lines)
		if quoted, requoted, err = s.requoteShipping(ctx, o, shipping, items, shipSubtotal, store.CurrencyCode); err != nil {
			return Order{}, err
		}
	}

	oldAmountDue := o.AmountDue()
	var updated Order
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		// Re-check under the order lock - the plan above comes from an unlocked read, so a shipment, a cancel or another edit committing meanwhile was edited over: a shipped line dropped, stock re-held on a cancelled order, totals from a line set that no longer existed
		if err := lockForEdit(ctx, r, o); err != nil {
			return err
		}
		if updated, err = r.UpdateOrderAddresses(ctx, o.ID, billing, shipping); err != nil {
			return err
		}
		// Only a shipping move changes money - a billing-only edit keeps the totals
		if in.Shipping != nil {
			if err := r.SetShipmentShipTo(ctx, o.ID, shipping); err != nil {
				return err
			}
			discount := min(o.DiscountAmount, subtotal)
			synthetic := syntheticCart(o, o.Lines)
			shipAmount := o.ShippingAmount
			if requoted {
				shipAmount = quoted
				if err := r.SetShipmentAmount(ctx, o.ID, shipAmount); err != nil {
					return err
				}
			}
			tax, shippingTax := o.TaxAmount, o.ShippingTaxAmount
			var taxLines []TaxLine
			if s.tax != nil {
				cc := &checkoutContext{customer: cust, shipping: shipping, store: &store, cart: synthetic, subtotal: subtotal}
				if tax, err = s.computeLineTax(ctx, cc, discount, shipAmount); err != nil {
					return err
				}
				taxLines, shippingTax = cc.taxLines, cc.shippingTax
			}
			total := computeOrderTotal(subtotal, discount, tax, shipAmount, store.TaxInclusivePricing, nil)
			if updated, err = r.UpdateOrderAmounts(ctx, o.ID, OrderAmounts{Subtotal: subtotal, Shipping: shipAmount, Discount: discount, Tax: tax, ShippingTax: shippingTax, Total: total}); err != nil {
				return err
			}
			if err := replaceTaxLines(ctx, r, o.ID, taxLines); err != nil {
				return err
			}
		}
		what := "billing"
		if in.Shipping != nil && in.Billing != nil {
			what = "shipping + billing"
		} else if in.Shipping != nil {
			what = "shipping"
		}
		if _, err := r.AddOrderEvent(ctx, OrderEvent{
			OrderID: o.ID,
			Type:    "order_address_edited",
			Message: fmt.Sprintf("%s address updated (%s, %s).", what, shipping.City, shipping.CountryCode),
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		return nil
	})
	if err != nil {
		return Order{}, err
	}
	newAmountDue := updated.AmountDue()
	if s.bus != nil && newAmountDue != oldAmountDue {
		s.publishDurable(ctx, &OrderEdited{OrderID: o.ID, ChannelID: o.ChannelID, OldAmountDue: oldAmountDue, NewAmountDue: newAmountDue})
	}
	return s.repo.GetOrderByID(ctx, o.ID)
}

// requoteShipping re-prices an order's shipping for an edit, ahead of the transaction so a carrier's round-trip never holds the order lock - lockForEdit refuses the edit if the order changed in between. ok is false when shipping holds: no method on record, several packages (each priced with its own method at its own destination, which the order's one carrier and method can't re-quote), or the carrier down or the method gone - an edit should not abort on an API blip, and the OrderEdited event surfaces any change
func (s *Service) requoteShipping(ctx context.Context, o Order, to OrderAddress, items []shippingpkg.RateItem, subtotal int64, currency string) (int64, bool, error) {
	if s.shipping == nil || o.ShippingCarrierSlug == nil || *o.ShippingCarrierSlug == "" || o.ShippingMethodCode == nil {
		return 0, false, nil
	}
	if single, err := s.singlePackage(ctx, o.ID); err != nil || !single {
		return 0, false, err
	}
	req := shippingpkg.RateRequest{
		ChannelID:     o.ChannelID,
		Destination:   shippingpkg.Address{CountryCode: to.CountryCode, Region: ptr.Deref(to.State), PostalCode: to.PostalCode},
		SubtotalCents: subtotal - min(o.DiscountAmount, subtotal),
		CurrencyCode:  currency,
		Items:         items,
	}
	cents, _, err := s.shipping.PriceSelected(ctx, *o.ShippingCarrierSlug, *o.ShippingMethodCode, req)
	if err != nil {
		log.Printf("[order] edit re-quote shipping for order %s failed; holding it at %d: %v", o.ID, o.ShippingAmount, err)
		return 0, false, nil
	}
	return cents, true, nil
}

func lockForEdit(ctx context.Context, r Repository, read Order) error {
	if err := r.LockOrder(ctx, read.ID); err != nil {
		return fmt.Errorf("lock order: %w", err)
	}
	fresh, err := r.GetOrderByID(ctx, read.ID)
	if err != nil {
		return err
	}
	if !editableStatuses[fresh.Status] {
		return apperr.New("ORDER_NOT_EDITABLE", apperr.ClassConflict,
			fmt.Sprintf("an order in status %q cannot be edited", fresh.Status))
	}
	if !fresh.UpdatedAt.Equal(read.UpdatedAt) {
		return apperr.New("ORDER_CHANGED", apperr.ClassConflict, "the order changed while this edit was being prepared - reload it and try again")
	}
	return nil
}
