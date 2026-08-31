package promotion

import (
	"sort"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/money"
)

// The mutation surface for actions - methods rather than fields, so the engine can audit each plugin's contributions and the concrete struct's allocation strategy can change without touching action implementations
type Ledger interface {
	AddSubtotalDiscount(cents int64)
	AddLineAdjustment(lineID uuid.UUID, cents int64)
	SetFreeShipping(methodSlugs []string)
	SetShippingOverride(cents int64)
	// qty > 0 GRANTS the gift, and order placement turns it into a zero-priced line with stock reserved
	// qty == 0 OFFERS it - core.add_free_gift_choice queues its whole candidate list that way, and the cart surfaces them as pickable without committing anything
	AddFreeGift(productSKU string, qty float64)
	// Clamps the running subtotal discount to maxCents, used by core.cap_total_discount - per-line allocations rescale proportionally, since the cap is on the aggregate rather than any single line
	// The cap is also REMEMBERED and re-applied once every rule has run, so a cap rule can't be defeated by another rule discounting after it - see ledgerImpl.Finalize
	CapSubtotalDiscount(maxCents int64)

	SubtotalDiscountCents() int64
	LineAdjustments() map[uuid.UUID]int64
	LineAdjustment(lineID uuid.UUID) int64
	FreeShipping() bool
	FreeShippingMethods() []string
	ShippingOverride() *int64
	FreeGifts() []FreeGift
}

type ledgerImpl struct {
	subtotalDiscount int64
	lineAdjust       map[uuid.UUID]int64
	freeShipping     bool
	freeShipMethods  []string
	shippingOverride *int64
	freeGifts        []FreeGift
	// The tightest ceiling any core.cap_total_discount action asked for, nil when none did - re-applied in Finalize so the cap holds regardless of which rule ran when
	discountCap *int64
}

func newLedger() *ledgerImpl {
	return &ledgerImpl{
		lineAdjust: make(map[uuid.UUID]int64),
	}
}

func (l *ledgerImpl) AddSubtotalDiscount(cents int64) {
	if cents <= 0 {
		return
	}
	l.subtotalDiscount += cents
}

func (l *ledgerImpl) AddLineAdjustment(lineID uuid.UUID, cents int64) {
	if cents <= 0 || lineID == uuid.Nil {
		return
	}
	l.lineAdjust[lineID] += cents
	l.subtotalDiscount += cents
}

func (l *ledgerImpl) SetFreeShipping(methodSlugs []string) {
	l.freeShipping = true
	l.freeShipMethods = methodSlugs
}

func (l *ledgerImpl) SetShippingOverride(cents int64) {
	v := max(cents, 0)
	l.shippingOverride = &v
}

// A zero qty is kept, not dropped - it's how an unpicked candidate is represented, and rejecting it made core.add_free_gift_choice a silent no-op
func (l *ledgerImpl) AddFreeGift(productSKU string, qty float64) {
	if productSKU == "" || qty < 0 {
		return
	}
	l.freeGifts = append(l.freeGifts, FreeGift{ProductSKU: productSKU, Quantity: qty})
}

// The recorded value is what Finalize re-applies, so a cap holds even when a later rule discounts after it - clamping here too keeps a plugin reading SubtotalDiscountCents mid-evaluation seeing a capped number
func (l *ledgerImpl) CapSubtotalDiscount(maxCents int64) {
	if maxCents < 0 {
		maxCents = 0
	}
	if l.discountCap == nil || maxCents < *l.discountCap {
		v := maxCents
		l.discountCap = &v
	}
	l.clampTo(maxCents)
}

// The shortfall between the rescaled per-line sum and maxCents is deliberately left UNallocated - AllocateLineDiscounts redistributes it weighted by line subtotal, which is the only place that knows line sizes
// Handing it to one line instead dumps the whole cart-level share onto whichever line sorted last, so a $8 cart-wide discount capped at $5 landed 90% on one line and refunds derived from it drifted
func (l *ledgerImpl) clampTo(maxCents int64) {
	if maxCents < 0 {
		maxCents = 0
	}
	original := l.subtotalDiscount
	if original <= maxCents {
		return
	}
	l.subtotalDiscount = maxCents
	ids := make([]uuid.UUID, 0, len(l.lineAdjust))
	for id := range l.lineAdjust {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, id := range ids {
		l.lineAdjust[id] = (l.lineAdjust[id] * maxCents) / original
	}
}

// Re-applies any cap_total_discount ceiling (so rule ordering can't defeat it), then clamps to the cart subtotal (so stacked percent rules can't discount more than the cart is worth), then spreads what's left across the lines
// Without the subtotal clamp two 60%-off rules stacked to 120%: the order total floored at zero but discount_amount, the tax base and every per-line refund were computed from a number larger than the cart
func (l *ledgerImpl) Finalize(lines []CartLineView, subtotalCents int64) {
	if l.discountCap != nil {
		l.clampTo(*l.discountCap)
	}
	ceiling := subtotalCents
	if ceiling <= 0 {
		// Simulations and tests build an environment from lines alone - fall back to what the lines are worth
		for _, line := range lines {
			ceiling += money.LineTotal(line.UnitPriceCents, line.Quantity)
		}
	}
	l.clampTo(ceiling)
	l.AllocateLineDiscounts(lines)
}

func (l *ledgerImpl) SubtotalDiscountCents() int64         { return l.subtotalDiscount }
func (l *ledgerImpl) LineAdjustments() map[uuid.UUID]int64 { return l.lineAdjust }
func (l *ledgerImpl) LineAdjustment(id uuid.UUID) int64    { return l.lineAdjust[id] }
func (l *ledgerImpl) FreeShipping() bool                   { return l.freeShipping }
func (l *ledgerImpl) FreeShippingMethods() []string        { return l.freeShipMethods }
func (l *ledgerImpl) ShippingOverride() *int64             { return l.shippingOverride }
func (l *ledgerImpl) FreeGifts() []FreeGift                { return l.freeGifts }

// Distributes subtotal-level discounts across lines proportionally to each line's subtotal contribution - without it, per-line refunds can't compute the post-discount unit price and EU-VAT tax-after-discount sees the wrong base
// The portion already assigned to specific lines through AddLineAdjustment stays put, and the unallocated remainder distributes weighted by pre-discount subtotal contribution, with rounding errors landing on the last line that has something to discount
func (l *ledgerImpl) AllocateLineDiscounts(lines []CartLineView) {
	if l.subtotalDiscount == 0 || len(lines) == 0 {
		return
	}
	alreadyAllocated := int64(0)
	for _, cents := range l.lineAdjust {
		alreadyAllocated += cents
	}
	remainder := l.subtotalDiscount - alreadyAllocated
	if remainder <= 0 {
		return
	}
	subs := make([]int64, len(lines))
	var grossSubtotal int64
	for i, line := range lines {
		subs[i] = money.LineTotal(line.UnitPriceCents, line.Quantity)
		grossSubtotal += subs[i]
	}
	if grossSubtotal <= 0 {
		return
	}
	distributed := int64(0)
	last := -1
	for i, line := range lines {
		share := (remainder * subs[i]) / grossSubtotal
		distributed += share
		if share > 0 {
			l.lineAdjust[line.LineID] += share
		}
		if subs[i] > 0 {
			last = i
		}
	}
	// Floor division never over-allocates, so the shortfall is ≥0 - it goes to the last line worth something, since a zero-priced granted gift would be discounted below nothing
	if short := remainder - distributed; short > 0 && last >= 0 {
		l.lineAdjust[lines[last].LineID] += short
	}
}
