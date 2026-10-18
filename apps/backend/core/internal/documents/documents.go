// Package documents renders order paperwork as PDFs: invoices (money document), packing slips (warehouse pick document, no prices), and
// credit memos (refund record) - Layout is deliberately plain - clean typography over branding, printable on any A4/letter printer
package documents

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// The PDF core fonts only cover Windows-1252, so Đorđe, Čačak or any Cyrillic name printed as mojibake - fpdf subsets these into each document
var (
	//go:embed fonts/DejaVuSansCondensed.ttf
	fontRegular []byte
	//go:embed fonts/DejaVuSansCondensed-Bold.ttf
	fontBold []byte
	//go:embed fonts/DejaVuSansCondensed-Oblique.ttf
	fontItalic []byte
)

const family = "DejaVu"

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

type Input struct {
	Kind  Kind
	Order order.Order
	Store StoreInfo
	Memo  *order.CreditMemo // required for KindCreditMemo, ignored otherwise
	// The order currency's minor-unit digits - amounts are minor units, so a yen total isn't divided by 100
	Decimals int
	// The channel's prices carry their tax, so the tax is a portion of the total rather than added on top of it
	TaxInclusive bool
}

type doc struct {
	pdf *fpdf.Fpdf
	in  Input
}

// Render produces the PDF bytes for one order document
func Render(in Input) ([]byte, error) {
	if in.Kind == KindCreditMemo && in.Memo == nil {
		return nil, fmt.Errorf("credit memo document requires a memo")
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(family, "", fontRegular)
	pdf.AddUTF8FontFromBytes(family, "B", fontBold)
	pdf.AddUTF8FontFromBytes(family, "I", fontItalic)
	pdf.SetMargins(18, 16, 18)
	pdf.AddPage()
	d := doc{pdf: pdf, in: in}

	d.letterhead()
	d.addresses()
	switch in.Kind {
	case KindCreditMemo:
		d.memoLines()
		d.memoTotals()
	case KindPackingSlip:
		d.orderLines()
		d.footer("Contents checked by: ____________    Date: ____________")
	default:
		d.orderLines()
		d.orderTotals()
		d.footer("Thank you for your order.")
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render %s: %w", in.Kind, err)
	}
	return buf.Bytes(), nil
}

func (d doc) money(minor int64) string {
	return d.in.Order.CurrencyCode + " " + money.Format(minor, d.in.Decimals)
}

func quantity(q float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", q), "0"), ".")
}

// Cuts s to what fits a cell w wide, ending in an ellipsis - CellFormat never wraps, so a long name ran over the next column
func (d doc) fit(s string, w float64) string {
	if w == 0 { // fpdf's "to the right margin"
		pageW, _ := d.pdf.GetPageSize()
		_, _, right, _ := d.pdf.GetMargins()
		w = pageW - right - d.pdf.GetX()
	}
	room := w - 2*d.pdf.GetCellMargin()
	if d.pdf.GetStringWidth(s) <= room {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && d.pdf.GetStringWidth(string(r)+"…") > room {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func (d doc) cell(w, h float64, s, border string, ln int, align string, fill bool) {
	d.pdf.CellFormat(w, h, d.fit(s, w), border, ln, align, fill, 0, "")
}

func (d doc) letterhead() {
	pdf, o := d.pdf, d.in.Order
	pdf.SetFont(family, "B", 16)
	d.cell(0, 8, d.in.Store.Name, "", 1, "L", false)
	pdf.SetFont(family, "", 9)
	pdf.SetTextColor(90, 90, 90)
	for _, l := range d.in.Store.Lines {
		d.cell(0, 4.5, l, "", 1, "L", false)
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(4)

	pdf.SetFont(family, "B", 13)
	d.cell(0, 7, map[Kind]string{KindInvoice: "INVOICE", KindPackingSlip: "PACKING SLIP", KindCreditMemo: "CREDIT MEMO"}[d.in.Kind], "", 1, "L", false)
	pdf.SetFont(family, "", 9)
	if m := d.in.Memo; d.in.Kind == KindCreditMemo {
		d.cell(0, 5, fmt.Sprintf("%s   -   %s", m.Number, m.CreatedAt.Format("2 Jan 2006")), "", 1, "L", false)
	}
	d.cell(0, 5, fmt.Sprintf("Order %s   -   %s", o.Number, o.CreatedAt.Format("2 Jan 2006")), "", 1, "L", false)
	d.cell(0, 5, fmt.Sprintf("Generated %s", time.Now().UTC().Format("2 Jan 2006 15:04 MST")), "", 1, "L", false)
	pdf.Ln(3)
}

func addressLines(label string, a order.OrderAddress) []string {
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
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
	return append(lines, cityLine, a.CountryCode)
}

// The slip ships the parcel, so it carries only SHIP TO
func (d doc) addresses() {
	pdf, o := d.pdf, d.in.Order
	columns := [][]string{addressLines("BILL TO", o.BillingAddress), addressLines("SHIP TO", o.ShippingAddress)}
	if d.in.Kind == KindPackingSlip {
		columns = columns[1:]
	}
	startY, endY := pdf.GetY(), pdf.GetY()
	for i, lines := range columns {
		pdf.SetY(startY)
		x, w := 18.0, 90.0
		if i == 1 {
			x, w = 108, 84
		}
		for j, l := range lines {
			if j == 0 {
				pdf.SetFont(family, "B", 8)
				pdf.SetTextColor(120, 120, 120)
			} else {
				pdf.SetFont(family, "", 9)
				pdf.SetTextColor(0, 0, 0)
			}
			pdf.SetX(x)
			d.cell(w, 4.6, l, "", 1, "L", false)
		}
		endY = max(endY, pdf.GetY())
	}
	pdf.SetY(endY)
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(5)
}

func (d doc) tableHeader(priced bool) {
	d.pdf.SetFont(family, "B", 8)
	d.pdf.SetFillColor(243, 243, 243)
	if !priced {
		d.cell(36, 6, "SKU", "B", 0, "L", true)
		d.cell(118, 6, "Item", "B", 0, "L", true)
		d.cell(20, 6, "Qty", "B", 1, "R", true)
	} else {
		d.cell(30, 6, "SKU", "B", 0, "L", true)
		d.cell(76, 6, "Item", "B", 0, "L", true)
		d.cell(16, 6, "Qty", "B", 0, "R", true)
		d.cell(26, 6, "Unit", "B", 0, "R", true)
		d.cell(26, 6, "Total", "B", 1, "R", true)
	}
	d.pdf.SetFont(family, "", 9)
}

func (d doc) pricedRow(sku, name string, qty float64, unit, total int64) {
	d.cell(30, 6, sku, "B", 0, "L", false)
	d.cell(76, 6, name, "B", 0, "L", false)
	d.cell(16, 6, quantity(qty), "B", 0, "R", false)
	d.cell(26, 6, d.money(unit), "B", 0, "R", false)
	d.cell(26, 6, d.money(total), "B", 1, "R", false)
}

func (d doc) orderLines() {
	pdf := d.pdf
	priced := d.in.Kind != KindPackingSlip
	d.tableHeader(priced)
	for _, l := range d.in.Order.Lines {
		if priced {
			d.pricedRow(l.ProductSku, l.ProductName, l.Quantity, l.UnitPrice, l.TotalPrice)
		} else {
			d.cell(36, 6, l.ProductSku, "B", 0, "L", false)
			d.cell(118, 6, l.ProductName, "B", 0, "L", false)
			d.cell(20, 6, quantity(l.Quantity), "B", 1, "R", false)
		}
		// Custom-option picks: essential on the packing slip (the picker needs size/engraving/uploaded-file), informative on invoices
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
			pdf.SetFont(family, "I", 8)
			pdf.SetTextColor(90, 90, 90)
			d.cell(36, 4.5, "", "", 0, "L", false)
			d.cell(138, 4.5, "- "+opt.OptionTitle+": "+v, "", 1, "L", false)
			pdf.SetFont(family, "", 9)
			pdf.SetTextColor(0, 0, 0)
		}
	}
	pdf.Ln(3)
}

// A credit memo lists what it credited, at the quantities and unit basis it credited them at - never the order's own lines
func (d doc) memoLines() {
	byLineID := make(map[uuid.UUID]order.OrderLine, len(d.in.Order.Lines))
	for _, l := range d.in.Order.Lines {
		byLineID[l.ID] = l
	}
	d.tableHeader(true)
	for _, ml := range d.in.Memo.Lines {
		src := byLineID[ml.OrderLineID]
		d.pricedRow(src.ProductSku, src.ProductName, ml.Quantity, ml.UnitCents, ml.TotalCents)
	}
	if len(d.in.Memo.Lines) == 0 {
		d.pdf.SetTextColor(120, 120, 120)
		d.cell(0, 6, "No items credited - this memo is an adjustment only.", "B", 1, "L", false)
		d.pdf.SetTextColor(0, 0, 0)
	}
	d.pdf.Ln(3)
}

func (d doc) totalRow(label string, minor int64, bold bool) {
	if bold {
		d.pdf.SetFont(family, "B", 10)
	} else {
		d.pdf.SetFont(family, "", 9)
	}
	d.pdf.SetX(120)
	d.cell(40, 5.5, label, "", 0, "L", false)
	d.cell(34, 5.5, d.money(minor), "", 1, "R", false)
}

// Gift cards and store credit pay part of the total rather than lowering it - tax was charged on the full price - so they come after it
func (d doc) orderTotals() {
	o := d.in.Order
	d.totalRow("Subtotal", o.SubtotalAmount, false)
	if o.DiscountAmount > 0 {
		d.totalRow("Discount", -o.DiscountAmount, false)
	}
	d.totalRow("Shipping", o.ShippingAmount, false)
	if !d.in.TaxInclusive {
		d.totalRow("Tax", o.TaxAmount, false)
	}
	d.totalRow("Total", o.TotalAmount, true)
	if d.in.TaxInclusive {
		d.totalRow("Includes tax", o.TaxAmount, false)
	}
	if o.GiftCardAmount > 0 || o.StoreCreditAmount > 0 {
		if o.GiftCardAmount > 0 {
			d.totalRow("Gift card", -o.GiftCardAmount, false)
		}
		if o.StoreCreditAmount > 0 {
			d.totalRow("Store credit", -o.StoreCreditAmount, false)
		}
		d.totalRow("Amount due", o.AmountDue(), true)
	}
}

// A credit memo closes out with its own arithmetic - the order's totals belong on the invoice, not here
func (d doc) memoTotals() {
	m := d.in.Memo
	d.totalRow("Items", m.LinesTotalCents, false)
	if m.ShippingRefundCents > 0 {
		d.totalRow("Shipping", m.ShippingRefundCents, false)
	}
	if m.AdjustmentRefundCents > 0 {
		d.totalRow("Adjustment", m.AdjustmentRefundCents, false)
	}
	if m.AdjustmentFeeCents > 0 {
		d.totalRow("Fee", -m.AdjustmentFeeCents, false)
	}
	d.totalRow("Refunded", m.TotalCents, true)

	where := "refunded to the original payment method"
	switch {
	case m.RefundTarget == order.RefundTargetGiftCard:
		where = "issued as a gift card"
	case m.Offline:
		where = "refunded outside the payment gateway"
	}
	note := fmt.Sprintf("Credit memo %s - %s.", m.Number, where)
	if m.Reason != "" {
		note += " " + m.Reason
	}
	d.footer(note)
}

func (d doc) footer(note string) {
	d.pdf.Ln(6)
	d.pdf.SetFont(family, "", 8)
	d.pdf.SetTextColor(120, 120, 120)
	d.cell(0, 5, note, "", 1, "L", false)
}
