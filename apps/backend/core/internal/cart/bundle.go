package cart

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

type AddBundleSelectionInput struct { // SelectionIDs is at most one for radio and dropdown, zero-or-more for checkbox and multiselect
	OptionID     uuid.UUID
	SelectionIDs []uuid.UUID
}

type AddBundleToCartInput struct { // the bundle counterpart to AddCartLineInput, one Selections entry per option the customer picked for
	ProductSku string
	Quantity   float64
	Selections []AddBundleSelectionInput
	Variants   map[uuid.UUID]uuid.UUID // configurable selection -> the variant picked for it
	Quantities map[uuid.UUID]float64   // customer-quantity selection -> how many go in each bundle
}

// What a pick carries beyond its selection id - the variant of a configurable, the amount of a customer-quantity selection
type pickExtras struct {
	Variants   map[uuid.UUID]uuid.UUID
	Quantities map[uuid.UUID]float64
}

func (s *Service) AddBundleToCart(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, input AddBundleToCartInput) (Cart, error) { // prices the line from the picked components - re-adding the same SKU REPLACES the prior selection set
	if s.products == nil {
		return Cart{}, errNoCatalog
	}
	if err := validQuantity(input.Quantity); err != nil {
		return Cart{}, err
	}
	cart, err := authorizeLineEdit(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	if err := checkProtectedOfferLine(cart, uuid.Nil, input.ProductSku); err != nil {
		return Cart{}, err
	}
	snap, err := s.products.ResolveBundleBySKU(ctx, cart.ChannelID, input.ProductSku)
	if errors.Is(err, ErrProductNotPublished) {
		return Cart{}, err
	}
	if err != nil {
		return Cart{}, fmt.Errorf("%w: %w", ErrProductNotBundle, err)
	}
	picked, err := bundlePicks(snap, input.Selections, pickExtras{Variants: input.Variants, Quantities: input.Quantities})
	if err != nil {
		return Cart{}, err
	}

	components, perBundle, sum, err := s.priceBundle(ctx, cart, picked)
	if err != nil {
		return Cart{}, err
	}
	unitPrice := snap.UnitPrice(sum)
	priced := unitPrice

	bundleName := snap.ProductName
	if bundleName == "" {
		bundleName = input.ProductSku
	}
	bundleQty := input.Quantity
	if s.bus != nil { // the pre-commit hook may reject the bundle or override the summed price and the quantity
		adding := &BundleAdding{
			CartID:      cart.ID,
			ChannelID:   cart.ChannelID,
			CustomerID:  cart.CustomerID,
			ProductSku:  input.ProductSku,
			ProductName: bundleName,
			UnitPrice:   &unitPrice,
			Quantity:    &bundleQty,
		}
		for i, c := range components {
			adding.StockParts = append(adding.StockParts, CartLineStockPart{ProductID: c.ProductID, Quantity: perBundle[i]})
		}
		if err := s.bus.PublishSync(ctx, adding); err != nil {
			return Cart{}, fmt.Errorf("add bundle to cart rejected: %w", err)
		}
		unitPrice, bundleQty = valueOr(adding.UnitPrice, unitPrice), valueOr(adding.Quantity, bundleQty) // read off the event, since the SDK setters and a webhook response swap the pointer rather than write through it
	}
	hookPriced := unitPrice != priced
	for i := range components {
		components[i].Quantity = perBundle[i] * bundleQty
	}
	var bundleLine CartLine
	if err := s.inTx(ctx, func(r Repository) error {
		var err error
		bundleLine, err = r.UpsertBundleCartLine(ctx, cart.ID, AddBundleLineInput{
			ProductName: bundleName,
			ProductSku:  input.ProductSku,
			UnitPrice:   unitPrice,
			Quantity:    bundleQty,
			Components:  components,
			HookPriced:  hookPriced,
			ConfigKey:   bundleConfigKey(components, perBundle),
		})
		return err
	}); err != nil {
		return Cart{}, fmt.Errorf("add bundle to cart: %w", err)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &BundleAdded{
			CartID:     cart.ID,
			ChannelID:  cart.ChannelID,
			CustomerID: cart.CustomerID,
			AnonID:     auth.AnonID(ctx),
			LineID:     bundleLine.ID,
			ProductSku: input.ProductSku,
			UnitPrice:  unitPrice,
			Quantity:   bundleQty,
		})
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

// Prices each pick at its channel price and sums them - the bundle's pricing mode turns the sum into its unit price, and perBundle is each component's quantity per bundle unit, left for the caller to scale once the line quantity is final
func (s *Service) priceBundle(ctx context.Context, c Cart, picked []pickedSelection) (components []AddCartLineComponentInput, perBundle []float64, unitPrice int64, err error) {
	for _, sel := range picked {
		compUnit := sel.UnitPrice // the snapshot's catalog default misses listing overrides, specials and remote pricing, so a channel-priced deployment would charge the wrong bundle total - fail-open back to it so a pricing hiccup never blocks the add
		sp, perr := s.resolvePricing(ctx, c.ChannelID, sel.ProductSKU, c.CustomerID)
		switch {
		case perr == nil:
			compUnit = sp.UnitPrice
		case errors.Is(perr, ErrProductNotPublished), errors.Is(perr, ErrProductNotPurchasable):
			return nil, nil, 0, fmt.Errorf("bundle component %s: %w", sel.ProductSKU, perr)
		}
		compUnit -= (compUnit*int64(sel.DiscountPercent) + 50) / 100 // an add-on deal comes off the component, rounded half up like the bundle's own percent
		components = append(components, AddCartLineComponentInput{
			BundleOptionID:    sel.optionID,
			BundleSelectionID: sel.ID,
			ProductID:         sel.ProductID,
			ProductSku:        sel.ProductSKU,
			ProductName:       sel.ProductName,
			UnitPrice:         compUnit,
			SortOrder:         int32(len(components)),
		})
		perBundle = append(perBundle, sel.QuantityPerBundle)
		unitPrice += money.LineTotal(compUnit, sel.QuantityPerBundle)
	}
	return components, perBundle, unitPrice, nil
}

// A stored bundle line re-resolved against today's bundle - component prices, and quantities per bundle, as add-to-cart would set them now. ok is false when the line can't be re-resolved (the bundle or a pick is gone, or a component left the channel), which checkout refuses separately
// A line the cart.bundle.adding hook priced keeps that price, the plugin's to keep - only its components refresh. Every other line takes today's pricing mode over today's sum
func (s *Service) liveBundleLine(ctx context.Context, c Cart, l CartLine) (unitPrice int64, components []AddCartLineComponentInput, ok bool) {
	snap, err := s.products.ResolveBundleBySKU(ctx, c.ChannelID, l.ProductSku)
	if err != nil {
		return 0, nil, false
	}
	picked, err := bundlePicks(snap, picksFromComponents(snap, l.Components), extrasFromLine(l))
	if err != nil {
		return 0, nil, false
	}
	components, perBundle, sum, err := s.priceBundle(ctx, c, picked)
	if err != nil {
		return 0, nil, false
	}
	for i := range components {
		components[i].Quantity = perBundle[i] * l.Quantity
	}
	if l.BundleHookPriced {
		return l.UnitPrice, components, true
	}
	return snap.UnitPrice(sum), components, true
}

// Whether the re-resolved line differs from what the cart stored, in price or in any component's price, quantity or identity
func bundleLineDrifted(l CartLine, unitPrice int64, components []AddCartLineComponentInput) bool {
	if unitPrice != l.UnitPrice || len(components) != len(l.Components) {
		return true
	}
	for i, c := range components {
		was := l.Components[i]
		if c.BundleSelectionID != was.BundleSelectionID || c.UnitPrice != was.UnitPrice || math.Abs(c.Quantity-was.Quantity) > 1e-9 {
			return true
		}
	}
	return false
}

// Names a set of picks - selection, the product it holds (a configurable's variant) and how many per bundle - so the
// same picks always land on the same line and different ones on a line of their own
func bundleConfigKey(comps []AddCartLineComponentInput, perBundle []float64) string {
	parts := make([]string, len(comps))
	for i, c := range comps {
		parts[i] = fmt.Sprintf("%s:%s:%g", c.BundleSelectionID, c.ProductID, perBundle[i])
	}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

type pickedSelection struct {
	BundleSelectionSnapshot
	optionID uuid.UUID
}

// Resolves the customer's picks against the bundle's own structure, in option order - each option at most once and each selection at most once, since a repeat would slip a second pick past a single-select or count one pick twice toward min_selections
// Untouched options take their defaults, and a supplied option always wins, even an explicit empty list, so a customer can deselect an optional default
func bundlePicks(snap BundleSnapshot, inputs []AddBundleSelectionInput, extras pickExtras) ([]pickedSelection, error) {
	optByID := make(map[uuid.UUID]BundleOptionSnapshot, len(snap.Options))
	for _, o := range snap.Options {
		optByID[o.ID] = o
	}
	supplied := make(map[uuid.UUID][]uuid.UUID, len(inputs))
	for _, e := range inputs {
		opt, ok := optByID[e.OptionID]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrBundleOptionUnknown, e.OptionID)
		}
		if _, dup := supplied[e.OptionID]; dup {
			return nil, fmt.Errorf("%w: option %s supplied twice", ErrBundleSelectionInvalid, e.OptionID)
		}
		seen := make(map[uuid.UUID]bool, len(e.SelectionIDs))
		for _, id := range e.SelectionIDs {
			if seen[id] {
				return nil, fmt.Errorf("%w: selection %s picked twice on option %s", ErrBundleSelectionInvalid, id, opt.ID)
			}
			seen[id] = true
		}
		if isSingleSelectInput(opt.InputType) && len(e.SelectionIDs) > 1 {
			return nil, fmt.Errorf("%w: option %s expected at most 1 selection, got %d", ErrBundleSelectionCardinality, opt.ID, len(e.SelectionIDs))
		}
		supplied[e.OptionID] = e.SelectionIDs
	}

	var out []pickedSelection
	for _, opt := range snap.Options {
		ids, ok := supplied[opt.ID]
		if !ok {
			for _, sel := range opt.Selections {
				if !sel.IsDefault {
					continue
				}
				ids = append(ids, sel.ID)
				if isSingleSelectInput(opt.InputType) {
					break
				}
			}
		}
		count := int32(len(ids))
		effMin := opt.MinSelections // raised to 1 when required
		if opt.IsRequired && effMin < 1 {
			effMin = 1
		}
		if count < effMin {
			if effMin == 1 {
				return nil, fmt.Errorf("%w: option %s", ErrBundleSelectionMissing, opt.ID)
			}
			return nil, fmt.Errorf("%w: option %s needs at least %d selections, got %d", ErrBundleSelectionCardinality, opt.ID, effMin, count)
		}
		if opt.MaxSelections != nil && count > *opt.MaxSelections {
			return nil, fmt.Errorf("%w: option %s allows at most %d selections, got %d", ErrBundleSelectionCardinality, opt.ID, *opt.MaxSelections, count)
		}
		for _, id := range ids {
			sel, found := selectionOn(opt, id)
			if !found {
				return nil, fmt.Errorf("%w: selection %s on option %s", ErrBundleSelectionInvalid, id, opt.ID)
			}
			if sel.Variants != nil {
				v, ok := variantOf(sel, extras.Variants[sel.ID])
				if !ok {
					return nil, fmt.Errorf("%w: selection %s", ErrBundleVariantMissing, sel.ID)
				}
				// The component is the variant from here on - its SKU prices, its stock is reserved, its name is on the order
				sel.ProductID, sel.ProductSKU, sel.ProductName, sel.UnitPrice = v.ProductID, v.ProductSKU, v.ProductName, v.UnitPrice
			}
			// Only a customer-quantity selection takes the shopper's amount - any other keeps the bundle's, so a stored line follows a merchant's change
			if q, ok := extras.Quantities[sel.ID]; ok && sel.CustomerQuantity {
				if q <= 0 || math.IsNaN(q) || math.IsInf(q, 0) {
					return nil, fmt.Errorf("%w: selection %s needs a quantity above zero", ErrBundleSelectionInvalid, sel.ID)
				}
				sel.QuantityPerBundle = q
			}
			out = append(out, pickedSelection{BundleSelectionSnapshot: sel, optionID: opt.ID})
		}
	}
	if len(out) == 0 { // a line with no components prices at zero and reads as a simple line downstream, so checkout would let it through for free
		return nil, fmt.Errorf("%w: the bundle needs at least one selection", ErrBundleSelectionMissing)
	}
	return out, nil
}

// Rebuilds a line's picks from its component rows, with every live option supplied - an option the line has nothing for is an explicit empty pick, so a since-required option fails instead of quietly taking today's defaults, and an option that no longer exists is still sent so bundlePicks rejects it
func picksFromComponents(snap BundleSnapshot, comps []CartLineComponent) []AddBundleSelectionInput {
	byOption := make(map[uuid.UUID][]uuid.UUID, len(snap.Options))
	var order []uuid.UUID
	for _, o := range snap.Options {
		byOption[o.ID] = nil
		order = append(order, o.ID)
	}
	for _, c := range comps {
		if _, ok := byOption[c.BundleOptionID]; !ok {
			order = append(order, c.BundleOptionID)
		}
		byOption[c.BundleOptionID] = append(byOption[c.BundleOptionID], c.BundleSelectionID)
	}
	out := make([]AddBundleSelectionInput, 0, len(order))
	for _, id := range order {
		out = append(out, AddBundleSelectionInput{OptionID: id, SelectionIDs: byOption[id]})
	}
	return out
}

func variantOf(sel BundleSelectionSnapshot, id uuid.UUID) (BundleVariantSnapshot, bool) {
	for _, v := range sel.Variants {
		if v.ProductID == id {
			return v, true
		}
	}
	return BundleVariantSnapshot{}, false
}

// A stored line's extras per selection - the component row holds the variant and the amount, which get rechecked against today's bundle
func extrasFromLine(l CartLine) pickExtras {
	out := pickExtras{Variants: make(map[uuid.UUID]uuid.UUID, len(l.Components)), Quantities: make(map[uuid.UUID]float64, len(l.Components))}
	for _, c := range l.Components {
		out.Variants[c.BundleSelectionID] = c.ProductID
		if l.Quantity > 0 {
			out.Quantities[c.BundleSelectionID] = c.Quantity / l.Quantity // rows hold line totals
		}
	}
	return out
}

func selectionOn(opt BundleOptionSnapshot, id uuid.UUID) (BundleSelectionSnapshot, bool) {
	for _, sel := range opt.Selections {
		if sel.ID == id {
			return sel, true
		}
	}
	return BundleSelectionSnapshot{}, false
}

func componentInputs(comps []CartLineComponent) []AddCartLineComponentInput { // a stored line's components as upsert input, cloned verbatim
	out := make([]AddCartLineComponentInput, len(comps))
	for i, c := range comps {
		out[i] = AddCartLineComponentInput{
			BundleOptionID:    c.BundleOptionID,
			BundleSelectionID: c.BundleSelectionID,
			ProductID:         c.ProductID,
			ProductSku:        c.ProductSku,
			ProductName:       c.ProductName,
			UnitPrice:         c.UnitPrice,
			Quantity:          c.Quantity,
			SortOrder:         c.SortOrder,
		}
	}
	return out
}
