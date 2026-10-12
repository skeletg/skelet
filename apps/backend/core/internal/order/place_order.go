package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/address"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/money"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	shippingpkg "github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// The resolved pre-transaction state for a PlaceOrder call, produced by resolveCheckout and read by the tx and post-commit steps - one struct instead of threading a dozen values through every helper
type checkoutContext struct {
	customer        customer.Customer
	billing         OrderAddress
	shipping        OrderAddress
	store           *channel.Channel
	paymentProvider string
	cart            cart.Cart
	subtotal        int64
	orderID         uuid.UUID // pre-allocated so pre-tx subscribers can link writes
	taxLines        []TaxLine // per-rule breakdown from the last computeLineTax call
	shippingTax     int64
}

// Loads and validates everything PlaceOrder needs before the tx - customer, addresses, channel country and region enforcement, payment provider, the cart with its ownership checks and subtotal, and a pre-allocated order id
func (s *Service) resolveCheckout(ctx context.Context, in PlaceOrderInput) (*checkoutContext, error) {
	if in.Email == "" {
		return nil, apperr.New("EMAIL_REQUIRED", apperr.ClassValidation, "email is required")
	}

	// Load the customer if any (guests skip this)
	var c customer.Customer
	if in.CustomerID != nil {
		loaded, err := s.customerSvc.GetByIDWithAddresses(ctx, *in.CustomerID)
		if err != nil {
			return nil, fmt.Errorf("load customer: %w", err)
		}
		c = loaded
	}

	// Resolve billing + shipping - inline wins, an ID lookup needs a logged-in customer with that address
	billing, err := resolveAddress(in.BillingAddress, in.BillingAddressID, in.CustomerID != nil, c, "billing")
	if err != nil {
		return nil, err
	}
	shipping, err := resolveAddress(in.ShippingAddress, in.ShippingAddressID, in.CustomerID != nil, c, "shipping")
	if err != nil {
		return nil, err
	}

	// PlaceOrder is storefront-only - without a store there's no currency to stamp
	store, ok := channel.FromContext(ctx)
	if !ok {
		return nil, ErrStoreRequired
	}

	// Enforce the channel's country/region options on both addresses, no-op without a validator
	if s.countries != nil {
		if err := s.countries.ValidateAddressForChannel(ctx, store.ID, shipping.CountryCode, shipping.State); err != nil {
			return nil, fmt.Errorf("shipping address: %w", err)
		}
		if err := s.countries.ValidateAddressForChannel(ctx, store.ID, billing.CountryCode, billing.State); err != nil {
			return nil, fmt.Errorf("billing address: %w", err)
		}
	}

	if err := s.checkStrictAddresses(ctx, store.ID, shipping, billing); err != nil {
		return nil, err
	}

	// Resolve + validate the payment provider - optional (no slug = no provider),
	// but when supplied it must be enabled for the channel and accept the shipping country
	var paymentProvider string
	if in.PaymentProviderSlug != nil {
		paymentProvider = strings.TrimSpace(*in.PaymentProviderSlug)
	}
	if paymentProvider != "" && s.payments != nil {
		if err := s.payments.ValidatePaymentForChannel(ctx, store.ID, paymentProvider, shipping.CountryCode); err != nil {
			return nil, fmt.Errorf("payment method: %w", err)
		}
	}

	cartObj, err := s.resolveCheckoutCart(ctx, in, store.ID)
	if err != nil {
		return nil, err
	}
	if len(cartObj.Lines) == 0 {
		return nil, apperr.New("CART_EMPTY", apperr.ClassValidation, "cart is empty")
	}

	// Reprice BEFORE the subtotal so promotions, tax and fraud all see today's prices - the fail-open contract lives in the repricer
	if s.repricer != nil {
		cartObj = s.repricer.RepriceCartLines(ctx, cartObj)
		if v, ok := s.repricer.(lineValidator); ok {
			if err := v.ValidateCheckoutLines(ctx, cartObj); err != nil {
				return nil, err
			}
		}
	}

	var subtotal int64
	for _, l := range cartObj.Lines {
		subtotal += money.LineTotal(l.UnitPrice, l.Quantity)
	}

	orderID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("allocate order id: %w", err)
	}

	return &checkoutContext{
		customer:        c,
		billing:         billing,
		shipping:        shipping,
		store:           store,
		paymentProvider: paymentProvider,
		cart:            cartObj,
		subtotal:        subtotal,
		orderID:         orderID,
	}, nil
}

// Only a fail_closed provider's outage refuses placement - any other validator failure lets the order through, same as the storefront
func (s *Service) checkStrictAddresses(ctx context.Context, channelID uuid.UUID, shipping, billing OrderAddress) error {
	if s.addresses == nil || !s.addresses.Strict(ctx, channelID) {
		return nil
	}
	ship, bill := addressInput(shipping), addressInput(billing)
	if err := s.checkStrictAddress(ctx, channelID, "shipping", ship); err != nil {
		return err
	}
	if bill == ship { // same address, already answered
		return nil
	}
	return s.checkStrictAddress(ctx, channelID, "billing", bill)
}

func (s *Service) checkStrictAddress(ctx context.Context, channelID uuid.UUID, which string, in address.Input) error {
	res, err := s.addresses.Validate(ctx, channelID, in)
	if errors.Is(err, address.ErrUnavailable) {
		return fmt.Errorf("%s address: %w", which, err) // ErrUnavailable is a registered sentinel, so the storefront gets its code
	}
	if err != nil {
		log.Printf("[order] address validator failed on channel %s, not blocking: %v", channelID, err)
		return nil
	}
	if res.Valid {
		return nil
	}
	msg := "Please check the " + which + " address"
	if len(res.Messages) > 0 {
		msg += ": " + strings.Join(res.Messages, "; ")
	}
	return apperr.New("ADDRESS_INVALID", apperr.ClassValidation, msg+".")
}

func addressInput(a OrderAddress) address.Input {
	return address.Input{
		Street1:     a.Street1,
		Street2:     ptr.Deref(a.Street2),
		City:        a.City,
		State:       ptr.Deref(a.State),
		PostalCode:  a.PostalCode,
		CountryCode: a.CountryCode,
	}
}

// resolveCheckoutCart loads the cart to place - an explicit id (checked for store
// match + ownership) is preferred, else a logged-in customer's active cart
func (s *Service) resolveCheckoutCart(ctx context.Context, in PlaceOrderInput, storeID uuid.UUID) (cart.Cart, error) {
	if in.CartID != nil {
		cartObj, err := s.cartRepo.GetCartByID(ctx, *in.CartID)
		if err != nil {
			return cart.Cart{}, fmt.Errorf("load cart: %w", err)
		}
		lines, err := s.cartRepo.ListCartLinesByCart(ctx, cartObj.ID)
		if err != nil {
			return cart.Cart{}, fmt.Errorf("load cart lines: %w", err)
		}
		cartObj.Lines = lines
		if cartObj.ChannelID != storeID {
			return cart.Cart{}, apperr.New("CART_WRONG_STORE", apperr.ClassValidation, "cart belongs to a different store")
		}
		// A guest can't checkout a customer's cart, nor a customer someone else's
		if cartObj.CustomerID != nil {
			if in.CustomerID == nil || *cartObj.CustomerID != *in.CustomerID {
				return cart.Cart{}, apperr.New("CART_FORBIDDEN", apperr.ClassForbidden, "cart belongs to another customer")
			}
		}
		return cartObj, nil
	}
	if in.CustomerID != nil {
		return cart.GetOrCreateCustomerCart(ctx, s.cartRepo, *in.CustomerID, storeID)
	}
	return cart.Cart{}, apperr.New("CART_REQUIRED", apperr.ClassValidation, "a cart is required")
}

// Runs the core per-line tax when no plugin claimed the seam - resolves each line's tax class by SKU, distributes the discount across lines, and asks the tax service for the total. totalDiscount floors at zero Also records the per-rule breakdown on cc.taxLines - the engine resolves it anyway, and dropping it left orders with a tax_amount nothing could explain
func (s *Service) computeLineTax(ctx context.Context, cc *checkoutContext, totalDiscount, shippingCents int64) (int64, error) {
	cc.taxLines, cc.shippingTax = nil, 0
	// Tax-exempt customers pay zero core tax on every recompute path - a TaxCents plugin is still authoritative, this only gates core
	if cc.customer.TaxExempt {
		return 0, nil
	}
	if totalDiscount < 0 {
		totalDiscount = 0
	}
	region := ""
	if cc.shipping.State != nil {
		region = *cc.shipping.State
	}

	classByLine := make([]string, len(cc.cart.Lines))
	if s.taxClasses != nil {
		skus := make([]string, 0, len(cc.cart.Lines))
		for _, l := range cc.cart.Lines {
			skus = append(skus, l.ProductSku)
		}
		classes, err := s.taxClasses.ProductTaxClassesBySKUs(ctx, skus)
		if err != nil {
			return 0, fmt.Errorf("resolve tax classes: %w", err)
		}
		for i, l := range cc.cart.Lines {
			classByLine[i] = classes[l.ProductSku]
		}
	}

	lines := allocateLineTaxInputs(cc.cart.Lines, cc.subtotal, totalDiscount, classByLine)
	goods := len(lines)
	if shippingCents > 0 {
		class, taxable := "", true
		if s.shippingTax != nil {
			class, taxable = s.shippingTax(ctx, cc.store.ID)
		}
		if taxable {
			lines = append(lines, tax.LineTaxInput{ProductTaxClass: class, TaxableCents: shippingCents})
		}
	}
	res, err := s.tax.ComputeOrder(ctx, tax.ComputeOrderInput{
		ChannelID:         cc.store.ID,
		CountryCode:       cc.shipping.CountryCode,
		RegionCode:        region,
		TaxInclusive:      cc.store.TaxInclusivePricing,
		CustomerGroupSlug: customerGroupSlug(cc.customer),
		Lines:             lines,
	})
	if err != nil {
		return 0, fmt.Errorf("compute tax: %w", err)
	}
	if len(res.Lines) > goods {
		cc.shippingTax = res.Lines[goods].TotalCents
	}
	// Sum per rule - the engine works line by line, but the document wants one row per tax
	byRule := map[uuid.UUID]*TaxLine{}
	order := []uuid.UUID{}
	for _, line := range res.Lines {
		for _, a := range line.Applied {
			existing, ok := byRule[a.RuleID]
			if !ok {
				ruleID := a.RuleID
				existing = &TaxLine{Name: a.Name, RateBPS: a.RateBPS}
				if ruleID != uuid.Nil {
					existing.RuleID = &ruleID
				}
				byRule[a.RuleID] = existing
				order = append(order, a.RuleID)
			}
			existing.AmountCents += a.Cents
		}
	}
	cc.taxLines = make([]TaxLine, 0, len(order))
	for _, id := range order {
		cc.taxLines = append(cc.taxLines, *byRule[id])
	}
	return res.TotalCents, nil
}

// Distributes a whole-order discount across lines proportional to each subtotal in integer cents, with the remainder landing on the last non-empty line so shares sum exactly
// Pure, and shared by the placement tax math and the return-refund basis so both round identically
func allocateLineDiscount(lineSubs []int64, subtotal, totalDiscount int64) []int64 {
	out := make([]int64, len(lineSubs))
	var allocated int64
	last := -1
	for i, lineSub := range lineSubs {
		var d int64
		if subtotal > 0 {
			d = totalDiscount * lineSub / subtotal
		}
		out[i] = d
		allocated += d
		if lineSub > 0 {
			last = i
		}
	}
	// Floor division never over-allocates, so the remainder ≥0 lands on the last non-empty line
	if rem := totalDiscount - allocated; rem > 0 && last >= 0 {
		out[last] += rem
	}
	return out
}

// Distributes the discount across lines and pairs each post-discount taxable amount with its tax class - pure, so table-testable, and classByLine must be len(lines)
func allocateLineTaxInputs(lines []cart.CartLine, subtotal, totalDiscount int64, classByLine []string) []tax.LineTaxInput {
	lineSubs := make([]int64, len(lines))
	for i, l := range lines {
		lineSubs[i] = money.LineTotal(l.UnitPrice, l.Quantity)
	}
	discounts := allocateLineDiscount(lineSubs, subtotal, totalDiscount)
	out := make([]tax.LineTaxInput, len(lines))
	for i := range lines {
		taxable := max(lineSubs[i]-discounts[i], 0)
		out[i] = tax.LineTaxInput{ProductTaxClass: classByLine[i], TaxableCents: taxable}
	}
	return out
}

// The resolved shipping - priced cents plus the carrier and method to persist for an authoritative re-quote on edit - All-zero means free, a promo override, or no method available
type shippingSelection struct {
	Cents       int64
	CarrierSlug string
	MethodCode  string
	MethodID    *uuid.UUID // built-in carrier's method uuid, nil for external
}

// Prices shipping when no plugin claimed the seam - a free-shipping promo wins, then a shipping-override promo, then the chosen carrier and method re-priced server-side against tampering, else the cheapest across enabled carriers
// Returns the selection so an edit can re-quote
func (s *Service) resolveShippingCents(ctx context.Context, in PlaceOrderInput, cc *checkoutContext, discount int64, ranEngine bool, eval promotion.EvalResult) (shippingSelection, error) {
	req := rateRequestFromCheckout(cc, discount)

	slug, code := "", ""
	if in.ShippingCarrierSlug != nil {
		slug = *in.ShippingCarrierSlug
	}
	if in.ShippingMethodCode != nil {
		code = *in.ShippingMethodCode
	}
	// Free shipping can be restricted to named methods ("free standard, express excluded"). FreeShippingMethods was populated by both the built-in action and the remote provider but read by nobody, so the restriction was dead and express shipped free. An empty list still means every method. Returning an empty selection also blanks the carrier, which is why this resolves the chosen method FIRST - otherwise the order could never produce a label or a tracking URL.
	if ranEngine && eval.Ledger.FreeShipping() {
		allowed := eval.Ledger.FreeShippingMethods()
		if len(allowed) == 0 || slices.Contains(allowed, code) {
			return shippingSelection{CarrierSlug: slug, MethodCode: code}, nil
		}
	}
	if ranEngine && eval.Ledger.ShippingOverride() != nil {
		return shippingSelection{Cents: *eval.Ledger.ShippingOverride(), CarrierSlug: slug, MethodCode: code}, nil
	}
	// No explicit choice → aggregate-quote across enabled carriers, pick cheapest
	if slug == "" || code == "" {
		quotes, err := s.shipping.QuoteAll(ctx, req)
		if err != nil {
			return shippingSelection{}, fmt.Errorf("quote shipping: %w", err)
		}
		best, ok := cheapestRatedQuote(quotes)
		if !ok {
			return shippingSelection{}, nil // no method available → 0, like before
		}
		slug, code = best.CarrierSlug, best.Method.Code
	}
	// Authoritative re-price via the chosen carrier (fails closed if it's disabled or the method vanished)
	cents, methodID, err := s.shipping.PriceSelected(ctx, slug, code, req)
	if err != nil {
		return shippingSelection{}, fmt.Errorf("price shipping: %w", err)
	}
	return shippingSelection{Cents: cents, CarrierSlug: slug, MethodCode: code, MethodID: methodID}, nil
}

func rateRequestFromCheckout(cc *checkoutContext, discount int64) shippingpkg.RateRequest {
	items := make([]shippingpkg.RateItem, 0, len(cc.cart.Lines))
	for _, l := range cc.cart.Lines {
		items = append(items, shippingpkg.RateItem{SKU: l.ProductSku, Quantity: l.Quantity, UnitPriceCents: l.UnitPrice})
	}
	region := ""
	if cc.shipping.State != nil {
		region = *cc.shipping.State
	}
	return shippingpkg.RateRequest{
		ChannelID: cc.store.ID,
		Destination: shippingpkg.Address{
			CountryCode: cc.shipping.CountryCode,
			Region:      region,
			PostalCode:  cc.shipping.PostalCode,
		},
		SubtotalCents: cc.subtotal - discount,
		CurrencyCode:  cc.store.CurrencyCode,
		Items:         items,
	}
}

// Writes every cart line onto the order, copying bundle components with quantities already pre-multiplied and option picks, so the order is a complete immutable record of what was configured and paid for
func (s *Service) persistOrderLines(ctx context.Context, orderR Repository, orderID uuid.UUID, channelID uuid.UUID, lines []cart.CartLine) error {
	// Snapshot unit COGS at placement in ONE batch (was 3 queries/line) - Best-effort
	// - an unresolvable SKU is absent from the map and stored NULL, not a failure
	var costBySKU map[string]int64
	if s.costs != nil {
		skus := make([]string, 0, len(lines))
		for _, l := range lines {
			skus = append(skus, l.ProductSku)
		}
		if m, err := s.costs.ProductCostsBySKUs(ctx, skus); err == nil {
			costBySKU = m
		}
	}
	for _, l := range lines {
		var costCents *int64
		if c, ok := costBySKU[l.ProductSku]; ok && c > 0 {
			cc := c
			costCents = &cc
		}
		// Reuse the cart line's id - the reservations from the order.placing hook link order_line_id to it through a deferred FK checked at commit, and ConvertCart serializes placement so ids reach order_lines once
		// Draft lines have no id, so one is generated
		orderLine, err := orderR.AddOrderLine(ctx, OrderLine{
			ID:          l.ID,
			OrderID:     orderID,
			ProductName: l.ProductName,
			ProductSku:  l.ProductSku,
			UnitPrice:   l.UnitPrice,
			Quantity:    l.Quantity,
			TotalPrice:  money.LineTotal(l.UnitPrice, l.Quantity),
			CostCents:   costCents,
			IsGift:      l.IsGift,
		})
		if err != nil {
			return fmt.Errorf("add order line: %w", err)
		}
		for _, c := range l.Components {
			if _, err := orderR.AddOrderLineComponent(ctx, OrderLineComponent{
				OrderLineID:       orderLine.ID,
				BundleOptionID:    c.BundleOptionID,
				BundleSelectionID: c.BundleSelectionID,
				ProductID:         c.ProductID,
				ProductSku:        c.ProductSku,
				ProductName:       c.ProductName,
				UnitPrice:         c.UnitPrice,
				Quantity:          c.Quantity,
				SortOrder:         c.SortOrder,
			}); err != nil {
				return fmt.Errorf("add order line component: %w", err)
			}
		}
		if err := orderR.AddOrderLineStockParts(ctx, orderLine.ID, orderStockParts(l.StockParts)); err != nil {
			return err
		}
		for _, o := range l.Options {
			if _, err := orderR.AddOrderLineOption(ctx, OrderLineOption{
				OrderLineID:     orderLine.ID,
				ProductOptionID: o.ProductOptionID,
				OptionValueID:   o.OptionValueID,
				InputType:       o.InputType,
				OptionTitle:     o.OptionTitle,
				ValueLabel:      o.ValueLabel,
				TextValue:       o.TextValue,
				PriceDelta:      o.PriceDelta,
				Quantity:        o.Quantity,
				SortOrder:       o.SortOrder,
				FileURL:         o.FileURL,
			}); err != nil {
				return fmt.Errorf("add order line option: %w", err)
			}
		}
	}
	return nil
}

// Snapshots what each line takes stock from - a kit's current recipe, a configured line's linked option sources - so the order takes and later gives back the same stock even after the catalog changes
func (s *Service) stampStockParts(ctx context.Context, lines []cart.CartLine) ([]cart.CartLine, error) {
	out := make([]cart.CartLine, len(lines))
	copy(out, lines)
	for i := range out {
		l := &out[i]
		if len(l.Components) > 0 || len(l.StockParts) > 0 || l.ProductSku == "" {
			continue
		}
		if s.kits != nil {
			comps, err := s.kits.KitComponentsBySKU(ctx, l.ProductSku)
			if err != nil {
				return nil, fmt.Errorf("kit components for %q: %w", l.ProductSku, err)
			}
			l.StockParts = comps
		}
		l.StockParts = append(l.StockParts, cart.OptionStockParts(l.Options)...)
	}
	return out, nil
}

func orderStockParts(in []cart.CartLineStockPart) []OrderLineStockPart {
	out := make([]OrderLineStockPart, len(in))
	for i, p := range in {
		out[i] = OrderLineStockPart{ProductID: p.ProductID, Quantity: p.Quantity, Source: p.Source}
	}
	return out
}

// Redeems checkout gift-card codes against the total in the placement tx - each covers what it can, the provider takes the remainder, and payment_status flips to paid when they cover the whole total
func (s *Service) applyGiftCardTenders(ctx context.Context, sqlTx *sql.Tx, orderR Repository, orderID, channelID uuid.UUID, in PlaceOrderInput, total int64, currency string) (int64, error) {
	if s.giftcard == nil || len(in.GiftCardCodes) == 0 {
		return 0, nil
	}
	var giftCardApplied int64
	remaining := total
	for _, code := range in.GiftCardCodes {
		code = strings.TrimSpace(code)
		if code == "" || remaining <= 0 {
			continue
		}
		applied, err := s.giftcard.RedeemTx(ctx, sqlTx, code, channelID, remaining, currency, orderID)
		if err != nil {
			return 0, fmt.Errorf("redeem gift card: %w", err)
		}
		giftCardApplied += applied
		remaining -= applied
	}
	if giftCardApplied > 0 {
		paymentStatus := "unpaid"
		if giftCardApplied >= total {
			paymentStatus = "paid"
		}
		if _, err := orderR.SetGiftCardTender(ctx, orderID, giftCardApplied, paymentStatus); err != nil {
			return 0, fmt.Errorf("record gift card tender: %w", err)
		}
	}
	return giftCardApplied, nil
}

// Spends the customer's store-credit balance against what gift cards left uncovered, in the placement tx - opt-in through UseStoreCredit, and ignored for guests, who have no balance
func (s *Service) applyStoreCreditTender(ctx context.Context, sqlTx *sql.Tx, orderR Repository, orderID uuid.UUID, in PlaceOrderInput, total, alreadyTendered int64, currency string) (int64, error) {
	if s.storeCredit == nil || !in.UseStoreCredit || in.CustomerID == nil {
		return 0, nil
	}
	remaining := total - alreadyTendered
	if remaining <= 0 {
		return 0, nil
	}
	applied, err := s.storeCredit.RedeemTx(ctx, sqlTx, *in.CustomerID, remaining, currency, orderID)
	if err != nil {
		return 0, fmt.Errorf("redeem store credit: %w", err)
	}
	if applied <= 0 {
		return 0, nil
	}
	paymentStatus := "unpaid"
	if alreadyTendered+applied >= total {
		paymentStatus = "paid"
	}
	if _, err := orderR.SetStoreCreditTender(ctx, orderID, applied, paymentStatus); err != nil {
		return 0, fmt.Errorf("record store credit tender: %w", err)
	}
	return applied, nil
}

// Turns the engine's granted free gifts into zero-priced cart lines The Ledger carried these all along and nothing read them, so core.add_free_gift advertised a gift on the cart page that the placed order never contained and no stock was ever held for Skipped silently: gifts with no SKU, gifts whose SKU the catalog can't price (deleted, unlisted, or option-required, none of which can be auto-added), and gifts already in the cart, since a shopper who bought the item shouldn't get a second one free A qty of zero means "offered, not picked" - core.add_free_gift_choice queues its candidates that way and there's no picker yet, so those grant nothing
func (s *Service) appendFreeGiftLines(ctx context.Context, c cart.Cart, channelID uuid.UUID, gifts []promotion.FreeGift) cart.Cart {
	if len(gifts) == 0 || s.pricer == nil {
		return c
	}
	present := make(map[string]struct{}, len(c.Lines))
	for _, l := range c.Lines {
		present[l.ProductSku] = struct{}{}
		for _, comp := range l.Components {
			present[comp.ProductSku] = struct{}{}
		}
	}
	// Own the slice before appending - c is a copy but its backing array isn't, and growing the caller's array in place would leak gift lines into whatever else holds it
	lines := make([]cart.CartLine, len(c.Lines), len(c.Lines)+len(gifts))
	copy(lines, c.Lines)
	c.Lines = lines
	for _, g := range gifts {
		if g.ProductSKU == "" || g.Quantity <= 0 {
			continue
		}
		if _, dup := present[g.ProductSKU]; dup {
			continue
		}
		pricing, err := s.pricer.ResolveSimplePricing(ctx, channelID, g.ProductSKU)
		if err != nil {
			log.Printf("[promotion] free gift %q not grantable: %v", g.ProductSKU, err)
			continue
		}
		// Drop rather than block: appending a line inventory can't reserve would abort the whole placement off OrderPlacing,
		// so a promo offering a sold-out gift would lock every matching shopper out of checkout
		if s.giftStock != nil && !s.giftStock.CanFulfillSKU(ctx, channelID, g.ProductSKU, g.Quantity) {
			log.Printf("[promotion] free gift %q skipped: not salable", g.ProductSKU)
			continue
		}
		lineID, err := uuid.NewV7()
		if err != nil {
			log.Printf("[promotion] free gift %q: allocate line id: %v", g.ProductSKU, err)
			continue
		}
		present[g.ProductSKU] = struct{}{}
		c.Lines = append(c.Lines, cart.CartLine{
			ID:          lineID,
			CartID:      c.ID,
			ProductName: pricing.ProductName,
			ProductSku:  g.ProductSKU,
			UnitPrice:   0,
			Quantity:    g.Quantity,
			IsGift:      true,
		})
	}
	return c
}

// Increments usage counters, writes the audit row, fires the applied rules' RedemptionHooks and clears the cart's codes - all in the placement tx, so a failed order rolls the ledger debit back
func (s *Service) redeemPromotions(ctx context.Context, sqlTx *sql.Tx, cc *checkoutContext, in PlaceOrderInput, eval promotion.EvalResult) error {
	// Lines and email included - a loyalty or gift-card hook debiting in this tx needs to see what was actually bought, and an env without them made the whole cart invisible to it
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents:     cc.subtotal,
		Currency:          cc.store.CurrencyCode,
		CountryCode:       cc.shipping.CountryCode,
		ChannelID:         &cc.store.ID,
		ChannelSlug:       cc.store.Slug,
		CustomerID:        in.CustomerID,
		CustomerGroupSlug: customerGroupSlug(cc.customer),
		CustomerEmail:     in.Email,
		Lines:             cart.PromotionLineViews(cc.cart),
	})
	for _, ar := range eval.Applied {
		// Look up the rule's actions for the hook dispatch (fires per action type)
		rule, err := s.promotion.Repo().GetRule(ctx, ar.RuleID)
		if err != nil {
			return fmt.Errorf("load applied rule: %w", err)
		}
		actionsForHooks, err := s.promotion.Repo().ListActionsByRule(ctx, rule.ID)
		if err != nil {
			return fmt.Errorf("load actions for hooks: %w", err)
		}
		if err := s.promotion.Redeem(ctx, sqlTx, env, ar, in.CustomerID, cc.orderID, actionsForHooks); err != nil {
			return fmt.Errorf("redeem rule %s: %w", rule.Slug, err)
		}
	}
	if err := s.promotion.ClearCartCodes(ctx, sqlTx, cc.cart.ID); err != nil {
		return fmt.Errorf("clear cart codes: %w", err)
	}
	return nil
}

// Authorizes the amount owed after the order commits - best-effort, since a gateway failure can't roll a placed order back, so it records a visible "authorization failed" event and logs, leaving the order unpaid for retry
// The result settles payment_status asynchronously through the payment-to-order bridge
func (s *Service) authorizePostPlacement(ctx context.Context, in PlaceOrderInput, placed Order) PaymentAuthorizeOutcome {
	if s.paymentAuthorizer == nil || placed.PaymentProvider == "" {
		return PaymentAuthorizeOutcome{}
	}
	amountDue := placed.AmountDue()
	if amountDue <= 0 {
		return PaymentAuthorizeOutcome{}
	}
	token := ""
	if in.PaymentToken != nil {
		token = *in.PaymentToken
	}
	outcome, err := s.paymentAuthorizer.Authorize(ctx, PaymentAuthorizeInput{
		OrderID:      placed.ID,
		ChannelID:    placed.ChannelID,
		ProviderSlug: placed.PaymentProvider,
		AmountCents:  amountDue,
		CurrencyCode: placed.CurrencyCode,
		CustomerID:   placed.CustomerID,
		Email:        placed.Email,
		Token:        token,
		VaultedToken: in.VaultedPaymentToken,
	})
	if err != nil {
		log.Printf("[order] authorize payment for order %s (%s): %v", placed.ID, placed.PaymentProvider, err)
		if _, evErr := s.repo.AddOrderEvent(ctx, OrderEvent{
			OrderID: placed.ID,
			Type:    "payment_authorization_failed",
			Message: fmt.Sprintf("Payment authorization failed (%s): %v", placed.PaymentProvider, err),
		}); evErr != nil {
			log.Printf("[order] record authorization-failure event for order %s: %v", placed.ID, evErr)
		}
		return PaymentAuthorizeOutcome{}
	}
	// The hold is real once the gateway confirms it - record it, or every later capture is rejected against a zero authorized total. Offline providers return a zero outcome and correctly record nothing
	if outcome.Authorized {
		if _, aerr := s.RecordAuthorization(ctx, placed.ID, amountDue, placed.PaymentProvider, outcome.ProviderReference); aerr != nil {
			log.Printf("[order] record authorization ledger row for order %s: %v", placed.ID, aerr)
			// The gateway is holding the money and the ledger doesn't know - every later capture is rejected
			// against a zero authorized total, so this has to be visible on the order an operator actually opens
			if _, evErr := s.repo.AddOrderEvent(ctx, OrderEvent{
				OrderID: placed.ID,
				Type:    "payment_ledger_desync",
				Message: fmt.Sprintf("Payment (%s) was authorized at the gateway but the ledger row failed to write - capture will be refused until this is reconciled.", placed.PaymentProvider),
			}); evErr != nil {
				log.Printf("[order] record ledger-desync event for order %s: %v", placed.ID, evErr)
			}
		}
	}
	if outcome.RequiresAction() {
		// park the provider reference on the order - without it an abandoned redirect/3DS order carries
		// nothing the payment-sync sweep or finalizeOrderPayment could ask the gateway about
		if outcome.ProviderReference != "" {
			if _, perr := s.repo.SetPaymentReference(ctx, placed.ID, outcome.ProviderReference); perr != nil {
				log.Printf("[order] park provider reference for order %s: %v", placed.ID, perr)
				// Without the parked reference the sync sweep has nothing to ask the gateway about, so a
				// customer who completes 3DS is never settled - surface it on the timeline
				if _, evErr := s.repo.AddOrderEvent(ctx, OrderEvent{
					OrderID: placed.ID,
					Type:    "payment_reference_lost",
					Message: fmt.Sprintf("Payment (%s) needs customer authentication but its gateway reference could not be stored - this order will not settle automatically.", placed.PaymentProvider),
				}); evErr != nil {
					log.Printf("[order] record reference-lost event for order %s: %v", placed.ID, evErr)
				}
			}
		}
		if _, evErr := s.repo.AddOrderEvent(ctx, OrderEvent{
			OrderID: placed.ID,
			Type:    "payment_action_required",
			Message: fmt.Sprintf("Payment (%s) awaiting customer authentication (3DS).", placed.PaymentProvider),
		}); evErr != nil {
			log.Printf("[order] record payment-action event for order %s: %v", placed.ID, evErr)
		}
	}
	return outcome
}

type lineValidator interface { // bundle availability plus each line's min/max/step
	ValidateCheckoutLines(context.Context, cart.Cart) error
}
