package order

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// Prices the SKUs it knows and fails closed on anything else, like the catalog does
// for a deleted, unlisted or option-required product
type giftPricer map[string]string

func (g giftPricer) ResolveSimplePricing(_ context.Context, _ uuid.UUID, sku string) (SimplePricing, error) {
	name, ok := g[sku]
	if !ok {
		return SimplePricing{}, fmt.Errorf("no listing for %q", sku)
	}
	return SimplePricing{ProductName: name, UnitPriceCents: 4999}, nil
}

// A granted gift becomes a zero-priced line flagged as a gift, so placement writes it to order_lines and inventory reserves it off
// OrderPlacing - the Ledger carried these all along and nothing read them, so the cart advertised gifts the order never held
func TestAppendFreeGiftLines(t *testing.T) {
	channelID := uuid.New()
	base := cart.Cart{
		ID: uuid.New(),
		Lines: []cart.CartLine{
			{ID: uuid.New(), ProductSku: "SHIRT", UnitPrice: 2000, Quantity: 1},
		},
	}

	tests := []struct {
		name     string
		gifts    []promotion.FreeGift
		wantSkus []string
		wantQtys []float64
	}{
		{
			name:     "granted gift lands as a zero-priced line",
			gifts:    []promotion.FreeGift{{ProductSKU: "TOTE", Quantity: 1}},
			wantSkus: []string{"TOTE"},
			wantQtys: []float64{1},
		},
		{
			name:     "quantity carries through",
			gifts:    []promotion.FreeGift{{ProductSKU: "TOTE", Quantity: 3}},
			wantSkus: []string{"TOTE"},
			wantQtys: []float64{3},
		},
		{
			// core.add_free_gift_choice queues candidates at qty 0 - offered, not granted
			name:     "unpicked candidates grant nothing",
			gifts:    []promotion.FreeGift{{ProductSKU: "TOTE", Quantity: 0}, {ProductSKU: "MUG", Quantity: 0}},
			wantSkus: nil,
		},
		{
			name:     "a sku the catalog can't price is skipped",
			gifts:    []promotion.FreeGift{{ProductSKU: "DELETED", Quantity: 1}, {ProductSKU: "TOTE", Quantity: 1}},
			wantSkus: []string{"TOTE"},
			wantQtys: []float64{1},
		},
		{
			// The shopper already bought it, so no second one free - matches what the cart UI's inCart flag already told them
			name:     "a sku already in the cart is skipped",
			gifts:    []promotion.FreeGift{{ProductSKU: "SHIRT", Quantity: 1}},
			wantSkus: nil,
		},
		{
			name:     "the same gift queued twice grants once",
			gifts:    []promotion.FreeGift{{ProductSKU: "TOTE", Quantity: 1}, {ProductSKU: "TOTE", Quantity: 1}},
			wantSkus: []string{"TOTE"},
			wantQtys: []float64{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{pricer: giftPricer{"TOTE": "Canvas tote", "MUG": "Mug", "SHIRT": "Shirt"}}
			got := s.appendFreeGiftLines(context.Background(), base, channelID, tt.gifts)

			var giftLines []cart.CartLine
			for _, l := range got.Lines {
				if l.IsGift {
					giftLines = append(giftLines, l)
				}
			}
			if len(giftLines) != len(tt.wantSkus) {
				t.Fatalf("gift lines = %d, want %d (%+v)", len(giftLines), len(tt.wantSkus), giftLines)
			}
			for i, l := range giftLines {
				if l.ProductSku != tt.wantSkus[i] {
					t.Errorf("gift[%d].sku = %q, want %q", i, l.ProductSku, tt.wantSkus[i])
				}
				if l.Quantity != tt.wantQtys[i] {
					t.Errorf("gift[%d].qty = %g, want %g", i, l.Quantity, tt.wantQtys[i])
				}
				if l.UnitPrice != 0 {
					t.Errorf("gift[%d].unitPrice = %d, want 0", i, l.UnitPrice)
				}
				if l.ID == uuid.Nil {
					t.Errorf("gift[%d] has no line id - inventory reservations link to it", i)
				}
				if l.ProductName == "" {
					t.Errorf("gift[%d] has no product name", i)
				}
			}
			// The paid line is never disturbed
			if got.Lines[0].ProductSku != "SHIRT" || got.Lines[0].UnitPrice != 2000 {
				t.Errorf("paid line changed: %+v", got.Lines[0])
			}
		})
	}
}

// Without a pricer there's no way to name or validate the SKU, so nothing is granted
func TestAppendFreeGiftLinesNoPricer(t *testing.T) {
	s := &Service{}
	c := cart.Cart{ID: uuid.New()}
	got := s.appendFreeGiftLines(context.Background(), c, uuid.New(), []promotion.FreeGift{{ProductSKU: "TOTE", Quantity: 1}})
	if len(got.Lines) != 0 {
		t.Errorf("lines = %+v, want none", got.Lines)
	}
}

// A gift SKU stock can't cover is dropped, never appended - the reservation on OrderPlacing aborts the whole placement
// on a shortage, so a promo offering a sold-out gift would otherwise lock every matching shopper out of checkout
type giftStock map[string]bool

func (g giftStock) CanFulfillSKU(_ context.Context, _ uuid.UUID, sku string, _ float64) bool {
	return g[sku]
}

func TestAppendFreeGiftLinesSkipsUnstockable(t *testing.T) {
	s := &Service{
		pricer:    giftPricer{"TOTE": "Canvas tote", "MUG": "Mug"},
		giftStock: giftStock{"MUG": true},
	}
	c := cart.Cart{ID: uuid.New(), Lines: []cart.CartLine{{ID: uuid.New(), ProductSku: "SHIRT", UnitPrice: 2000, Quantity: 1}}}
	got := s.appendFreeGiftLines(context.Background(), c, uuid.New(),
		[]promotion.FreeGift{{ProductSKU: "TOTE", Quantity: 1}, {ProductSKU: "MUG", Quantity: 1}})

	var gifts []string
	for _, l := range got.Lines {
		if l.IsGift {
			gifts = append(gifts, l.ProductSku)
		}
	}
	if len(gifts) != 1 || gifts[0] != "MUG" {
		t.Errorf("gift lines = %v, want only MUG (TOTE is out of stock)", gifts)
	}
	// The paid order still goes through
	if len(got.Lines) != 2 {
		t.Errorf("lines = %d, want 2 (the shirt plus the one stockable gift)", len(got.Lines))
	}
}
