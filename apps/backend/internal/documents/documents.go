// Package documents renders order paperwork as PDFs: invoices (money document), packing slips (warehouse pick document, no prices), and
// credit memos (refund record) - Layout is deliberately plain - clean typography over branding, printable on any A4/letter printer
package documents

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/order"
)

// StoreInfo is the letterhead block (channel name + contact lines)
type StoreInfo struct {
	Name  string
	Lines []string // address / VAT id / support email, pre-formatted
}

// Kind selects the document flavor
type Kind string

const (
	KindInvoice     Kind = "invoice"
	KindPackingSlip Kind = "packing_slip"
	KindCreditMemo  Kind = "credit_memo"
)

// Render produces the PDF bytes for one order document
//
// A credit memo needs its memo (nil for the other kinds) and renders THAT memo:
// its own number, the lines and quantities it actually credited, and its own
// totals - summing every refund on the order into one document would print all
// order lines at their original quantities, so a partial refund would read like a full one
func Render(kind Kind, o order.Order, store StoreInfo, memo *order.CreditMemo) ([]byte, error) {
	if kind == KindCreditMemo && memo == nil {
		return nil, fmt.Errorf("credit memo document requires a memo")
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(18, 16, 18)
	pdf.AddPage()

	title := map[Kind]string{
		KindInvoice:     "INVOICE",
		KindPackingSlip: "PACKING SLIP",
		KindCreditMemo:  "CREDIT MEMO",
	}[kind]

	// Letterhead
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 8, store.Name, "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(90, 90, 90)
	for _, l := range store.Lines {
		pdf.CellFormat(0, 4.5, l, "", 1, "L", false, 0, "")
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(4)

	// Title + order meta
	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(0, 7, title, "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	if kind == KindCreditMemo {
		pdf.CellFormat(0, 5, fmt.Sprintf("%s   -   %s", memo.Number, memo.CreatedAt.Format("2 Jan 2006")), "", 1, "L", false, 0, "")
	}
	pdf.CellFormat(0, 5, fmt.Sprintf("Order %s   -   %s", o.Number, o.CreatedAt.Format("2 Jan 2006")), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, fmt.Sprintf("Generated %s", time.Now().UTC().Format("2 Jan 2006 15:04 MST")), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	// Addresses side by side
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	writeAddr := func(label string, a order.OrderAddress) []string {
		lines := []string{label, strings.TrimSpace(a.FirstName + " " + a.LastName)}
		if c := deref(a.Company); c != "" {
			lines = append(lines, c)
		}
		lines = append(lines, a.Street1)
		if s2 := deref(a.Street2); s2 != "" {
			lines = append(lines, s2)
		}
		cityLine := strings.TrimSpace(a.City + " " + a.PostalCode)
		if st := deref(a.State); st != "" {
			cityLine += ", " + st
		}
		lines = append(lines, cityLine, a.CountryCode)
		return lines
	}
	left := writeAddr("BILL TO", o.BillingAddress)
	right := writeAddr("SHIP TO", o.ShippingAddress)
	if kind == KindPackingSlip {
		left = right
		right = nil
	}
	startY := pdf.GetY()
	pdf.SetFont("Helvetica", "", 9)
	for i, l := range left {
		if i == 0 {
			pdf.SetFont("Helvetica", "B", 8)
			pdf.SetTextColor(120, 120, 120)
		} else {
			pdf.SetFont("Helvetica", "", 9)
			pdf.SetTextColor(0, 0, 0)
		}
		pdf.CellFormat(90, 4.6, l, "", 1, "L", false, 0, "")
	}
	if right != nil {
		endY := pdf.GetY()
		pdf.SetY(startY)
		for i, l := range right {
			if i == 0 {
				pdf.SetFont("Helvetica", "B", 8)
				pdf.SetTextColor(120, 120, 120)
			} else {
				pdf.SetFont("Helvetica", "", 9)
				pdf.SetTextColor(0, 0, 0)
			}
			pdf.SetX(108)
			pdf.CellFormat(84, 4.6, l, "", 1, "L", false, 0, "")
		}
		if pdf.GetY() < endY {
			pdf.SetY(endY)
		}
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(5)

	money := func(cents int64) string {
		return fmt.Sprintf("%s %.2f", o.CurrencyCode, float64(cents)/100)
	}

	// Line table
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(243, 243, 243)
	if kind == KindPackingSlip {
		pdf.CellFormat(36, 6, "SKU", "B", 0, "L", true, 0, "")
		pdf.CellFormat(118, 6, "Item", "B", 0, "L", true, 0, "")
		pdf.CellFormat(20, 6, "Qty", "B", 1, "R", true, 0, "")
	} else {
		pdf.CellFormat(30, 6, "SKU", "B", 0, "L", true, 0, "")
		pdf.CellFormat(76, 6, "Item", "B", 0, "L", true, 0, "")
		pdf.CellFormat(16, 6, "Qty", "B", 0, "R", true, 0, "")
		pdf.CellFormat(26, 6, "Unit", "B", 0, "R", true, 0, "")
		pdf.CellFormat(26, 6, "Total", "B", 1, "R", true, 0, "")
	}
	pdf.SetFont("Helvetica", "", 9)
	// A credit memo lists what it credited, at the quantities and unit basis it
	// credited them at - never the order's own lines
	if kind == KindCreditMemo {
		byLineID := make(map[uuid.UUID]order.OrderLine, len(o.Lines))
		for _, l := range o.Lines {
			byLineID[l.ID] = l
		}
		for _, ml := range memo.Lines {
			src := byLineID[ml.OrderLineID]
			qty := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", ml.Quantity), "0"), ".")
			pdf.CellFormat(30, 6, src.ProductSku, "B", 0, "L", false, 0, "")
			pdf.CellFormat(76, 6, src.ProductName, "B", 0, "L", false, 0, "")
			pdf.CellFormat(16, 6, qty, "B", 0, "R", false, 0, "")
			pdf.CellFormat(26, 6, money(ml.UnitCents), "B", 0, "R", false, 0, "")
			pdf.CellFormat(26, 6, money(ml.TotalCents), "B", 1, "R", false, 0, "")
		}
		if len(memo.Lines) == 0 {
			pdf.SetTextColor(120, 120, 120)
			pdf.CellFormat(0, 6, "No items credited - this memo is an adjustment only.", "B", 1, "L", false, 0, "")
			pdf.SetTextColor(0, 0, 0)
		}
		return renderMemoTotals(pdf, memo, money, o.CurrencyCode)
	}
	for _, l := range o.Lines {
		qty := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", l.Quantity), "0"), ".")
		if kind == KindPackingSlip {
			pdf.CellFormat(36, 6, l.ProductSku, "B", 0, "L", false, 0, "")
			pdf.CellFormat(118, 6, l.ProductName, "B", 0, "L", false, 0, "")
			pdf.CellFormat(20, 6, qty, "B", 1, "R", false, 0, "")
		} else {
			pdf.CellFormat(30, 6, l.ProductSku, "B", 0, "L", false, 0, "")
			pdf.CellFormat(76, 6, l.ProductName, "B", 0, "L", false, 0, "")
			pdf.CellFormat(16, 6, qty, "B", 0, "R", false, 0, "")
			pdf.CellFormat(26, 6, money(l.UnitPrice), "B", 0, "R", false, 0, "")
			pdf.CellFormat(26, 6, money(l.TotalPrice), "B", 1, "R", false, 0, "")
		}
		// Custom-option picks: essential on the packing slip (the picker
		// needs size/engraving/uploaded-file), informative on invoices
		for _, opt := range l.Options {
			v := opt.ValueLabel
			if v == "" {
				v = opt.TextValue
			}
			if v == "" && opt.FileURL != "" {
				v = "uploaded file: " + opt.FileURL
			}
			if v == "" {
				continue
			}
			pdf.SetFont("Helvetica", "I", 8)
			pdf.SetTextColor(90, 90, 90)
			pdf.CellFormat(36, 4.5, "", "", 0, "L", false, 0, "")
			pdf.CellFormat(138, 4.5, "- "+opt.OptionTitle+": "+v, "", 1, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 9)
			pdf.SetTextColor(0, 0, 0)
		}
	}
	pdf.Ln(3)

	// Money summary (invoice + credit memo only)
	if kind != KindPackingSlip {
		row := func(label string, cents int64, bold bool) {
			if bold {
				pdf.SetFont("Helvetica", "B", 10)
			} else {
				pdf.SetFont("Helvetica", "", 9)
			}
			pdf.SetX(120)
			pdf.CellFormat(40, 5.5, label, "", 0, "L", false, 0, "")
			pdf.CellFormat(34, 5.5, money(cents), "", 1, "R", false, 0, "")
		}
		row("Subtotal", o.SubtotalAmount, false)
		if o.DiscountAmount > 0 {
			row("Discount", -o.DiscountAmount, false)
		}
		row("Shipping", o.ShippingAmount, false)
		row("Tax", o.TaxAmount, false)
		if o.GiftCardAmount > 0 {
			row("Gift card", -o.GiftCardAmount, false)
		}
		row("Total", o.TotalAmount, true)
	}

	// Footer note
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(120, 120, 120)
	note := map[Kind]string{
		KindInvoice:     "Thank you for your order.",
		KindPackingSlip: "Contents checked by: ____________    Date: ____________",
	}[kind]
	pdf.CellFormat(0, 5, note, "", 1, "L", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render %s: %w", kind, err)
	}
	return buf.Bytes(), nil
}

// renderMemoTotals closes out a credit memo with its own arithmetic - the order's
// totals belong on the invoice, not here
func renderMemoTotals(pdf *fpdf.Fpdf, memo *order.CreditMemo, money func(int64) string, currency string) ([]byte, error) {
	pdf.Ln(3)
	row := func(label string, cents int64, bold bool) {
		if bold {
			pdf.SetFont("Helvetica", "B", 10)
		} else {
			pdf.SetFont("Helvetica", "", 9)
		}
		pdf.SetX(120)
		pdf.CellFormat(40, 5.5, label, "", 0, "L", false, 0, "")
		pdf.CellFormat(34, 5.5, money(cents), "", 1, "R", false, 0, "")
	}
	row("Items", memo.LinesTotalCents, false)
	if memo.ShippingRefundCents > 0 {
		row("Shipping", memo.ShippingRefundCents, false)
	}
	if memo.AdjustmentRefundCents > 0 {
		row("Adjustment", memo.AdjustmentRefundCents, false)
	}
	if memo.AdjustmentFeeCents > 0 {
		row("Fee", -memo.AdjustmentFeeCents, false)
	}
	row("Refunded", memo.TotalCents, true)

	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(120, 120, 120)
	where := "refunded to the original payment method"
	switch {
	case memo.RefundTarget == order.RefundTargetGiftCard:
		where = "issued as a gift card"
	case memo.Offline:
		where = "refunded outside the payment gateway"
	}
	note := fmt.Sprintf("Credit memo %s - %s.", memo.Number, where)
	if memo.Reason != "" {
		note += " " + memo.Reason
	}
	pdf.CellFormat(0, 5, note, "", 1, "L", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render credit memo: %w", err)
	}
	return buf.Bytes(), nil
}
