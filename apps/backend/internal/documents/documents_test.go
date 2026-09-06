package documents

import (
	"bytes"
	"compress/zlib"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// fpdf deflates its content streams, so asserting on what a document actually SAYS means inflating
// them first - without this the tests could only check that some bytes came out
func pdfText(t *testing.T, raw []byte) string {
	t.Helper()
	var out strings.Builder
	rest := raw
	for {
		i := bytes.Index(rest, []byte("stream"))
		if i < 0 {
			break
		}
		body := rest[i+len("stream"):]
		body = bytes.TrimLeft(body, "\r\n")
		j := bytes.Index(body, []byte("endstream"))
		if j < 0 {
			break
		}
		if r, err := zlib.NewReader(bytes.NewReader(body[:j])); err == nil {
			if inflated, err := io.ReadAll(r); err == nil {
				out.Write(inflated)
			}
			_ = r.Close()
		}
		rest = body[j:]
	}
	if out.Len() == 0 {
		t.Fatalf("no content stream could be inflated out of %d bytes of PDF", len(raw))
	}
	return out.String()
}

func str(s string) *string { return &s }

func testOrder() order.Order {
	return order.Order{
		ID:           uuid.New(),
		Number:       "ORD-1001",
		CurrencyCode: "USD",
		CreatedAt:    time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC),
		BillingAddress: order.OrderAddress{
			FirstName: "Ada", LastName: "Lovelace", Company: str("Analytical"),
			Street1: "1 Bridge St", City: "London", PostalCode: "E1 6AN", CountryCode: "GB",
		},
		ShippingAddress: order.OrderAddress{
			FirstName: "Grace", LastName: "Hopper",
			Street1: "9 Navy Yard", City: "Arlington", PostalCode: "22202", State: str("VA"), CountryCode: "US",
		},
		SubtotalAmount: 20000,
		DiscountAmount: 1500,
		ShippingAmount: 995,
		TaxAmount:      1650,
		GiftCardAmount: 500,
		TotalAmount:    20645,
		Lines: []order.OrderLine{
			{ID: uuid.New(), ProductSku: "TIRE-205", ProductName: "All-Season Tire", Quantity: 4, UnitPrice: 5000, TotalPrice: 20000,
				Options: []order.OrderLineOption{{ID: uuid.New(), OptionTitle: "Mounting", ValueLabel: "Yes"}}},
		},
	}
}

func testStore() StoreInfo {
	return StoreInfo{Name: "Skelet Tires", Lines: []string{"1 Depot Way", "VAT GB123456789"}}
}

func TestRenderProducesAValidPDFForEveryKind(t *testing.T) {
	o, store := testOrder(), testStore()
	memo := &order.CreditMemo{Number: "CM-1", CreatedAt: o.CreatedAt, TotalCents: 5000}

	for kind, m := range map[Kind]*order.CreditMemo{
		KindInvoice:     nil,
		KindPackingSlip: nil,
		KindCreditMemo:  memo,
	} {
		got, err := Render(kind, o, store, m)
		if err != nil {
			t.Errorf("Render(%s) = %v", kind, err)
			continue
		}
		if !bytes.HasPrefix(got, []byte("%PDF-")) {
			t.Errorf("Render(%s) did not produce a PDF header", kind)
		}
		if len(got) < 500 {
			t.Errorf("Render(%s) produced only %d bytes, want a real document", kind, len(got))
		}
	}
}

// The memo carries the numbers - rendering a credit memo without one would silently fall back to the
// order's own totals and print a full refund for a partial one
func TestCreditMemoWithoutAMemoIsRefused(t *testing.T) {
	if _, err := Render(KindCreditMemo, testOrder(), testStore(), nil); err == nil {
		t.Error("Render(credit_memo, nil memo) = nil, want an error")
	}
	// the other kinds neither need nor read one
	for _, kind := range []Kind{KindInvoice, KindPackingSlip} {
		if _, err := Render(kind, testOrder(), testStore(), nil); err != nil {
			t.Errorf("Render(%s) with no memo = %v, want nil", kind, err)
		}
	}
}

func TestInvoiceCarriesTheLetterheadOrderAndMoney(t *testing.T) {
	text := pdfText(t, mustRender(t, KindInvoice, testOrder(), testStore(), nil))

	for _, want := range []string{
		"INVOICE", "Skelet Tires", "VAT GB123456789",
		"ORD-1001", "TIRE-205", "All-Season Tire",
		"Subtotal", "Discount", "Shipping", "Tax", "Gift card", "Total",
		"USD 200.00", // subtotal
		"USD 206.45", // total
		"Thank you for your order",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the invoice does not mention %q", want)
		}
	}
}

// The picker in the warehouse works off this document, so prices must not be on it - a customer
// opening the box would otherwise see the price paid, and gift orders leak
func TestPackingSlipShowsNoPrices(t *testing.T) {
	text := pdfText(t, mustRender(t, KindPackingSlip, testOrder(), testStore(), nil))

	for _, want := range []string{"PACKING SLIP", "TIRE-205", "All-Season Tire", "Qty"} {
		if !strings.Contains(text, want) {
			t.Errorf("the packing slip does not mention %q", want)
		}
	}
	for _, unwanted := range []string{"USD", "Subtotal", "Total", "Tax", "Discount", "Unit", "Gift card"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("the packing slip leaks %q, want no money on a warehouse document", unwanted)
		}
	}
	if !strings.Contains(text, "Contents checked by") {
		t.Error("the packing slip is missing its check-off footer")
	}
}

// The slip ships the parcel, so it carries the SHIP TO address and drops the billing one entirely
func TestPackingSlipUsesTheShippingAddressOnly(t *testing.T) {
	text := pdfText(t, mustRender(t, KindPackingSlip, testOrder(), testStore(), nil))

	if !strings.Contains(text, "Grace Hopper") || !strings.Contains(text, "9 Navy Yard") {
		t.Error("the packing slip does not carry the shipping recipient")
	}
	if strings.Contains(text, "Ada Lovelace") || strings.Contains(text, "1 Bridge St") {
		t.Error("the packing slip carries the billing address, want only the shipping one")
	}
	if strings.Contains(text, "BILL TO") {
		t.Error("the packing slip renders a BILL TO block")
	}
}

func TestInvoiceCarriesBothAddresses(t *testing.T) {
	text := pdfText(t, mustRender(t, KindInvoice, testOrder(), testStore(), nil))
	for _, want := range []string{"BILL TO", "SHIP TO", "Ada Lovelace", "Analytical", "Grace Hopper", "Arlington 22202, VA"} {
		if !strings.Contains(text, want) {
			t.Errorf("the invoice does not carry %q", want)
		}
	}
}

// Custom-option picks are what the picker needs to pull the right variant, and they belong on both
// the slip and the invoice
func TestLineOptionsAreRendered(t *testing.T) {
	for _, kind := range []Kind{KindInvoice, KindPackingSlip} {
		text := pdfText(t, mustRender(t, kind, testOrder(), testStore(), nil))
		if !strings.Contains(text, "Mounting") || !strings.Contains(text, "Yes") {
			t.Errorf("%s does not render the line's custom option", kind)
		}
	}
}

// An option with no label falls back to the typed value, then to the uploaded file - a blank one is
// skipped rather than printed as an empty bullet
func TestLineOptionValueFallbacks(t *testing.T) {
	o := testOrder()
	o.Lines[0].Options = []order.OrderLineOption{
		{ID: uuid.New(), OptionTitle: "Engraving", TextValue: "For Grace"},
		{ID: uuid.New(), OptionTitle: "Artwork", FileURL: "https://cdn.invalid/art.png"},
		{ID: uuid.New(), OptionTitle: "Blank"},
	}
	text := pdfText(t, mustRender(t, KindPackingSlip, o, testStore(), nil))

	if !strings.Contains(text, "For Grace") {
		t.Error("an option with only a text value did not render it")
	}
	if !strings.Contains(text, "uploaded file") {
		t.Error("an option with only a file did not render the upload")
	}
	if strings.Contains(text, "Blank") {
		t.Error("an option with no value at all was rendered, want it skipped")
	}
}

// This is the documented fix: a memo prints ITS OWN lines and quantities, so a partial refund can't
// come out reading like a full one
func TestCreditMemoRendersTheMemosOwnLinesNotTheOrders(t *testing.T) {
	o := testOrder()
	o.Lines = append(o.Lines, order.OrderLine{
		ID: uuid.New(), ProductSku: "WIPER-A", ProductName: "Wiper Blade", Quantity: 2, UnitPrice: 1500, TotalPrice: 3000,
	})
	memo := &order.CreditMemo{
		Number: "CM-1001", CreatedAt: o.CreatedAt,
		Lines: []order.CreditMemoLine{
			{OrderLineID: o.Lines[0].ID, Quantity: 1, UnitCents: 5000, TotalCents: 5000}, // 1 of the 4 tires
		},
		LinesTotalCents: 5000,
		TotalCents:      5000,
	}

	text := pdfText(t, mustRender(t, KindCreditMemo, o, testStore(), memo))

	if !strings.Contains(text, "CREDIT MEMO") || !strings.Contains(text, "CM-1001") {
		t.Error("the memo does not carry its own number")
	}
	if !strings.Contains(text, "TIRE-205") {
		t.Error("the credited line is missing")
	}
	if strings.Contains(text, "WIPER-A") {
		t.Error("an order line the memo never credited was rendered, which is exactly the full-refund-looking document this guards against")
	}
	if !strings.Contains(text, "USD 50.00") {
		t.Error("the memo's own line total is missing")
	}
	if strings.Contains(text, "USD 206.45") {
		t.Error("the order's total appears on the memo, want only the memo's own arithmetic")
	}
	if !strings.Contains(text, "Refunded") {
		t.Error("the memo is missing its Refunded total row")
	}
}

// An adjustment-only memo has no lines at all, and an empty table would read as a rendering failure
func TestCreditMemoWithNoLinesSaysSo(t *testing.T) {
	memo := &order.CreditMemo{
		Number: "CM-1002", CreatedAt: testOrder().CreatedAt,
		AdjustmentRefundCents: 2500, TotalCents: 2500,
	}
	text := pdfText(t, mustRender(t, KindCreditMemo, testOrder(), testStore(), memo))

	if !strings.Contains(text, "No items credited") {
		t.Error("an adjustment-only memo does not explain its empty line table")
	}
	if !strings.Contains(text, "Adjustment") || !strings.Contains(text, "USD 25.00") {
		t.Error("the adjustment amount is missing from the totals")
	}
}

// A fee reduces what the customer gets back, so it has to print as a negative
func TestCreditMemoRendersAFeeAsADeduction(t *testing.T) {
	memo := &order.CreditMemo{
		Number: "CM-1003", CreatedAt: testOrder().CreatedAt,
		LinesTotalCents: 5000, AdjustmentFeeCents: 750, TotalCents: 4250,
	}
	text := pdfText(t, mustRender(t, KindCreditMemo, testOrder(), testStore(), memo))

	if !strings.Contains(text, "Fee") {
		t.Error("the fee row is missing")
	}
	if !strings.Contains(text, "USD -7.50") {
		t.Errorf("the fee did not render as a deduction, want %q in:\n%s", "USD -7.50", text)
	}
	if !strings.Contains(text, "USD 42.50") {
		t.Error("the net refunded total is missing")
	}
}

// Where the money went is the first thing a customer asks, so the footer has to name it
func TestCreditMemoFooterNamesTheRefundDestination(t *testing.T) {
	base := order.CreditMemo{Number: "CM-1004", CreatedAt: testOrder().CreatedAt, TotalCents: 1000}

	gift := base
	gift.RefundTarget = order.RefundTargetGiftCard
	if text := pdfText(t, mustRender(t, KindCreditMemo, testOrder(), testStore(), &gift)); !strings.Contains(text, "issued as a gift card") {
		t.Error("a gift-card refund does not say so")
	}

	offline := base
	offline.Offline = true
	if text := pdfText(t, mustRender(t, KindCreditMemo, testOrder(), testStore(), &offline)); !strings.Contains(text, "outside the payment gateway") {
		t.Error("an offline refund does not say so")
	}

	gateway := base
	gateway.Reason = "Damaged on arrival"
	text := pdfText(t, mustRender(t, KindCreditMemo, testOrder(), testStore(), &gateway))
	if !strings.Contains(text, "original payment method") {
		t.Error("a gateway refund does not name the destination")
	}
	if !strings.Contains(text, "Damaged on arrival") {
		t.Error("the refund reason is missing from the memo")
	}
}

// Money is stored in minor units, so a currency mix-up or an off-by-100 shows up here first
func TestMoneyFormattingIsMinorUnits(t *testing.T) {
	o := testOrder()
	o.CurrencyCode = "EUR"
	o.SubtotalAmount = 1
	o.DiscountAmount = 0
	o.ShippingAmount = 0
	o.TaxAmount = 0
	o.GiftCardAmount = 0
	o.TotalAmount = 123456789
	o.Lines = []order.OrderLine{{ID: uuid.New(), ProductSku: "S", ProductName: "N", Quantity: 1, UnitPrice: 1, TotalPrice: 1}}

	text := pdfText(t, mustRender(t, KindInvoice, o, testStore(), nil))
	if !strings.Contains(text, "EUR 0.01") {
		t.Error("one cent did not render as 0.01")
	}
	if !strings.Contains(text, "EUR 1234567.89") {
		t.Error("a large total did not render in major units")
	}
	if strings.Contains(text, "USD") {
		t.Error("the document hardcodes USD somewhere, want the order's currency")
	}
}

// A zero discount or gift card must not print a row - "Discount USD 0.00" reads as a failed promo
func TestZeroValuedSummaryRowsAreOmitted(t *testing.T) {
	o := testOrder()
	o.DiscountAmount = 0
	o.GiftCardAmount = 0

	text := pdfText(t, mustRender(t, KindInvoice, o, testStore(), nil))
	if strings.Contains(text, "Discount") {
		t.Error("a zero discount rendered a row")
	}
	if strings.Contains(text, "Gift card") {
		t.Error("a zero gift card rendered a row")
	}
	// shipping and tax always print, even at zero, because their absence is meaningful
	for _, want := range []string{"Shipping", "Tax", "Subtotal", "Total"} {
		if !strings.Contains(text, want) {
			t.Errorf("the summary is missing %q", want)
		}
	}
}

// Fractional quantities are real for weight-priced goods, and trailing zeros would read as noise
func TestQuantityFormattingTrimsTrailingZeros(t *testing.T) {
	o := testOrder()
	o.Lines = []order.OrderLine{
		{ID: uuid.New(), ProductSku: "WHOLE", ProductName: "Whole", Quantity: 4, UnitPrice: 100, TotalPrice: 400},
		{ID: uuid.New(), ProductSku: "PART", ProductName: "Part", Quantity: 1.5, UnitPrice: 100, TotalPrice: 150},
	}
	text := pdfText(t, mustRender(t, KindPackingSlip, o, testStore(), nil))

	if strings.Contains(text, "4.0000") || strings.Contains(text, "1.5000") {
		t.Errorf("quantities kept their trailing zeros in:\n%s", text)
	}
	if !strings.Contains(text, "1.5") {
		t.Error("a fractional quantity did not render")
	}
}

// An order with no lines still has to produce a document rather than fail the download
func TestRenderHandlesAnEmptyOrder(t *testing.T) {
	o := order.Order{Number: "ORD-EMPTY", CurrencyCode: "USD", CreatedAt: time.Now()}
	for _, kind := range []Kind{KindInvoice, KindPackingSlip} {
		got, err := Render(kind, o, StoreInfo{Name: "Store"}, nil)
		if err != nil {
			t.Errorf("Render(%s) on an empty order = %v, want a document", kind, err)
			continue
		}
		if !bytes.HasPrefix(got, []byte("%PDF-")) {
			t.Errorf("Render(%s) on an empty order did not produce a PDF", kind)
		}
	}
}

func mustRender(t *testing.T, kind Kind, o order.Order, store StoreInfo, memo *order.CreditMemo) []byte {
	t.Helper()
	got, err := Render(kind, o, store, memo)
	if err != nil {
		t.Fatalf("Render(%s): %v", kind, err)
	}
	return got
}
