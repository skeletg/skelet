package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/money"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// CreateDraftLine is one simple line of a draft, priced server-side (admins never set prices directly)
type CreateDraftLine struct {
	ProductSku string
	Quantity   float64
	// UnitPriceCents overrides the catalog price when non-nil (manual pricing, quote conversion)
	UnitPriceCents *int64
}

// CreateDraftOrderInput is an admin-built order - no cart (lines given directly),
// no payment token (drafts place unpaid), a manual discount instead of promo codes
type CreateDraftOrderInput struct {
	ChannelID           uuid.UUID
	CustomerID          *uuid.UUID
	Email               string
	BillingAddress      OrderAddress
	ShippingAddress     OrderAddress
	Lines               []CreateDraftLine
	ShippingCarrierSlug *string
	ShippingMethodCode  *string
	ManualDiscountCents int64
	CustomerNote        string
	// SourceCartID is the cart this was built from, marked converted in the same tx so it isn't double-processed
	SourceCartID *uuid.UUID
	// SkipRequiredOptionCheck bypasses the required-options gate for callers carrying personalization elsewhere (B2B quote)
	SkipRequiredOptionCheck bool
}

// CreateDraftOrder stages an admin order in `draft` - it prices (tax + shipping + manual discount) reusing the placement money machinery
// but holds NO stock, authorizes NO payment, fires NO events - Editable, then real via PlaceDraftOrder - Simple products only
func (s *Service) CreateDraftOrder(ctx context.Context, in CreateDraftOrderInput) (Order, error) {
	if in.Email == "" {
		return Order{}, apperr.New("EMAIL_REQUIRED", apperr.ClassValidation, "email is required")
	}
	if len(in.Lines) == 0 {
		return Order{}, apperr.New("DRAFT_EMPTY", apperr.ClassValidation, "a draft order needs at least one line")
	}
	if s.pricer == nil {
		return Order{}, apperr.New("DRAFT_UNAVAILABLE", apperr.ClassUnavailable, "draft orders are not configured (no pricer)")
	}

	billing, err := resolveAddress(&in.BillingAddress, nil, false, customer.Customer{}, "billing")
	if err != nil {
		return Order{}, err
	}
	shipping, err := resolveAddress(&in.ShippingAddress, nil, false, customer.Customer{}, "shipping")
	if err != nil {
		return Order{}, err
	}

	// Load the channel for currency + tax-inclusive config (admin context has no channel.FromContext)
	ch, err := s.channels.LoadChannel(ctx, in.ChannelID)
	if err != nil {
		return Order{}, fmt.Errorf("load channel: %w", err)
	}
	store := &ch

	// Enforce the channel's country/region options on both addresses, no-op without a validator
	if s.countries != nil {
		if err := s.countries.ValidateAddressForChannel(ctx, store.ID, shipping.CountryCode, shipping.State); err != nil {
			return Order{}, fmt.Errorf("shipping address: %w", err)
		}
		if err := s.countries.ValidateAddressForChannel(ctx, store.ID, billing.CountryCode, billing.State); err != nil {
			return Order{}, fmt.Errorf("billing address: %w", err)
		}
	}

	var cust customer.Customer
	if in.CustomerID != nil {
		loaded, lerr := s.customerSvc.GetByIDWithAddresses(ctx, *in.CustomerID)
		if lerr != nil {
			return Order{}, fmt.Errorf("load customer: %w", lerr)
		}
		cust = loaded
	}

	// Resolve each SKU to its catalog price/name - reject unknown/non-simple SKUs
	lines := make([]cart.CartLine, 0, len(in.Lines))
	var subtotal int64
	for _, dl := range in.Lines {
		if dl.Quantity <= 0 {
			return Order{}, apperr.New("DRAFT_INVALID_QTY", apperr.ClassValidation, "line quantity must be positive")
		}
		p, perr := s.pricer.ResolveSimplePricing(ctx, store.ID, dl.ProductSku)
		if perr != nil {
			return Order{}, fmt.Errorf("resolve price for %q: %w", dl.ProductSku, perr)
		}
		if !in.SkipRequiredOptionCheck {
			if err := s.rejectRequiredOptionSKUs(ctx, store.ID, dl.ProductSku); err != nil {
				return Order{}, err
			}
		}
		unit := p.UnitPriceCents
		if dl.UnitPriceCents != nil {
			if *dl.UnitPriceCents < 0 {
				return Order{}, apperr.New("DRAFT_INVALID_PRICE", apperr.ClassValidation, "line price override must not be negative")
			}
			unit = *dl.UnitPriceCents
		}
		lines = append(lines, cart.CartLine{
			ProductSku:  dl.ProductSku,
			ProductName: p.ProductName,
			UnitPrice:   unit,
			Quantity:    dl.Quantity,
		})
		subtotal += money.LineTotal(unit, dl.Quantity)
	}

	discount := min(max(in.ManualDiscountCents, 0), subtotal)

	// Reuse the placement pricing helpers via a synthetic checkout context
	cc := &checkoutContext{
		customer: cust,
		billing:  billing,
		shipping: shipping,
		store:    store,
		cart:     cart.Cart{ChannelID: store.ID, CustomerID: in.CustomerID, Lines: lines},
		subtotal: subtotal,
	}

	var taxCents int64
	if s.tax != nil {
		taxCents, err = s.computeLineTax(ctx, cc, discount)
		if err != nil {
			return Order{}, err
		}
	}

	var shipSel shippingSelection
	if s.shipping != nil {
		shipSel, err = s.resolveShippingCents(ctx, PlaceOrderInput{
			ShippingCarrierSlug: in.ShippingCarrierSlug,
			ShippingMethodCode:  in.ShippingMethodCode,
		}, cc, discount, false, promotion.EvalResult{})
		if err != nil {
			return Order{}, err
		}
	}

	total := computeOrderTotal(subtotal, discount, taxCents, shipSel.Cents, store.TaxInclusivePricing, nil)

	orderID, err := uuid.NewV7()
	if err != nil {
		return Order{}, fmt.Errorf("allocate order id: %w", err)
	}

	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		orderR := s.repo.WithTx(sqlTx)
		order, err := orderR.CreateOrder(ctx, Order{
			ID:                  orderID,
			CustomerID:          in.CustomerID,
			Email:               in.Email,
			ChannelID:           store.ID,
			Status:              OrderStatusDraft,
			PaymentStatus:       "unpaid",
			BillingAddress:      billing,
			ShippingAddress:     shipping,
			SubtotalAmount:      subtotal,
			DiscountAmount:      discount,
			TaxAmount:           taxCents,
			ShippingAmount:      shipSel.Cents,
			TotalAmount:         total,
			CurrencyCode:        store.CurrencyCode,
			CustomerNote:        in.CustomerNote,
			ShippingMethodID:    shipSel.MethodID,
			ShippingCarrierSlug: stringPtr(shipSel.CarrierSlug),
			ShippingMethodCode:  stringPtr(shipSel.MethodCode),
		})
		if err != nil {
			return fmt.Errorf("create draft order: %w", err)
		}
		if err := s.persistOrderLines(ctx, orderR, order.ID, in.ChannelID, lines); err != nil {
			return err
		}
		if _, err := orderR.AddOrderEvent(ctx, OrderEvent{
			OrderID: order.ID,
			Type:    "order_created_draft",
			Message: "Draft order created by admin.",
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		// Drafts price tax the same way checkout does, so they owe the same breakdown
		for _, tl := range cc.taxLines {
			if err := orderR.InsertTaxLine(ctx, order.ID, tl); err != nil {
				return err
			}
		}
		// Built from a cart - consume it in the same tx, already-converted is a soft success
		if in.SourceCartID != nil {
			if err := s.cartRepo.WithTx(sqlTx).ConvertCart(ctx, *in.SourceCartID); err != nil &&
				!errors.Is(err, cart.ErrCartAlreadyConverted) {
				return fmt.Errorf("convert source cart: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return Order{}, err
	}
	return s.repo.GetOrderByID(ctx, orderID)
}

// PlaceDraftOrder finalizes a draft into a pending order - reserves stock (a shortage aborts, leaving the draft intact), flips draft→pending, and dispatches OrderPlaced so side-effects fire as for a storefront placement - Places UNPAID - manual orders are paid afterward via the ledger
func (s *Service) PlaceDraftOrder(ctx context.Context, orderID uuid.UUID) (Order, error) {
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return Order{}, fmt.Errorf("order not found")
	}
	if o.Status != OrderStatusDraft {
		return Order{}, apperr.New("ORDER_NOT_DRAFT", apperr.ClassConflict, "only draft orders can be placed")
	}
	if !validTransition(o.Status, OrderStatusPending) {
		return Order{}, apperr.New("ORDER_INVALID_TRANSITION", apperr.ClassConflict, "draft cannot be placed")
	}

	var placed Order
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		orderR := s.repo.WithTx(sqlTx)
		// Reserve stock - a draft holds nothing, so this is a pure reserve, a shortage rolls the placement back
		if s.inventoryEditor != nil {
			if err := s.inventoryEditor.ReReserveForEditTx(ctx, sqlTx, o.ChannelID, o.ID, syntheticCart(o, o.Lines)); err != nil {
				return err
			}
		}
		updated, err := orderR.UpdateOrderStatusFrom(ctx, o.ID, OrderStatusDraft, OrderStatusPending)
		if err != nil {
			return fmt.Errorf("place draft: %w", err)
		}
		placed = updated
		if _, err := orderR.AddOrderEvent(ctx, OrderEvent{
			OrderID: o.ID,
			Type:    "order_placed",
			Message: "Draft order placed by admin.",
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		// Durable dispatch via the outbox - the event commits with the status change so a crash can't lose it
		if s.outbox != nil {
			if err := s.outbox.Enqueue(ctx, sqlTx, &OrderPlaced{
				OrderID:     o.ID,
				OrderNumber: o.Number,
				CustomerID:  o.CustomerID,
				Email:       o.Email,
				ChannelID:   o.ChannelID,
				TotalCents:  o.TotalAmount,
				Currency:    o.CurrencyCode,
			}); err != nil {
				return fmt.Errorf("enqueue order.placed: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return Order{}, err
	}

	// Fire-and-forget fallback for deployments without the outbox
	if s.outbox == nil && s.bus != nil {
		s.bus.Publish(ctx, &OrderPlaced{
			OrderID:     placed.ID,
			OrderNumber: placed.Number,
			CustomerID:  placed.CustomerID,
			Email:       placed.Email,
			ChannelID:   placed.ChannelID,
			TotalCents:  placed.TotalAmount,
			Currency:    placed.CurrencyCode,
		})
	}

	return s.repo.GetOrderByID(ctx, orderID)
}

// rejectRequiredOptionSKUs blocks admin entry of SKUs with required options (drafts/edits carry no picks)
func (s *Service) rejectRequiredOptionSKUs(ctx context.Context, channelID uuid.UUID, sku string) error {
	oap, ok := s.pricer.(OptionAwarePricer)
	if !ok {
		return nil
	}
	required, err := oap.HasRequiredOptions(ctx, channelID, sku)
	if err != nil {
		// Fail closed - skipping the gate on a catalog hiccup would let a required-option SKU onto an order with no picks
		return fmt.Errorf("resolve product options for %q: %w", sku, err)
	}
	if required {
		return apperr.New("ORDER_LINE_OPTIONS_REQUIRED", apperr.ClassValidation,
			fmt.Sprintf("%s requires product options - build this order from a configured cart instead", sku))
	}
	return nil
}
