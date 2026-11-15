package plugin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/documents"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func TestADocumentRendererGetsTheDocumentAsCoreWouldPrintIt(t *testing.T) {
	var got sdk.DocumentRequest
	sdk.NewPluginForTest("docs-test").RegisterDocumentRenderer(sdk.DocumentCreditMemo, func(_ context.Context, req sdk.DocumentRequest) ([]byte, error) {
		got = req
		return req.Default()
	})
	line := order.OrderLine{ID: uuid.New(), ProductSku: "COAT-1", ProductName: "Wool coat", Quantity: 2, UnitPrice: 9000, TotalPrice: 18000,
		Options: []order.OrderLineOption{{OptionTitle: "Size", ValueLabel: "M"}, {OptionTitle: "Engraving", TextValue: "Ada"}}}
	company := "Analytical GmbH"
	in := documents.Input{
		Kind: documents.KindCreditMemo, Decimals: 2, Note: "Reverse charge", Footer: "Thanks",
		Store: documents.StoreInfo{Name: "Skelet Apparel", Lines: []string{"Hauptstrasse 1"}},
		Order: order.Order{ChannelID: uuid.New(), Number: "ORD-1", CurrencyCode: "EUR", Lines: []order.OrderLine{line},
			BillingAddress: order.OrderAddress{FirstName: "Ada", Company: &company, CountryCode: "DE"},
			BusinessIDs:    []order.BusinessID{{Key: "vat", Label: "VAT number", Value: "DE123"}}},
		Memo: &order.CreditMemo{Number: "CM-1", TotalCents: 9000, Lines: []order.CreditMemoLine{{OrderLineID: line.ID, Quantity: 1, UnitCents: 9000, TotalCents: 9000}}},
	}
	pdf, ok, err := DocumentRender(context.Background(), in, func() ([]byte, error) { return documents.Render(in) })
	if !ok || err != nil || len(pdf) == 0 {
		t.Fatalf("ok %v, %v", ok, err)
	}
	m := got.Memo
	if m == nil || len(m.Lines) != 1 || m.Lines[0].Name != "Wool coat" || m.Lines[0].Quantity != 1 || len(m.Lines[0].Options) != 2 {
		t.Errorf("memo %+v", m)
	}
	if got.Order.Billing.Company != company || got.Order.BusinessIDs[0].Value != "DE123" || got.Seller.Note != "Reverse charge" || got.Seller.Details[0] != "Hauptstrasse 1" {
		t.Errorf("request %+v", got)
	}
	if _, ok, _ := DocumentRender(context.Background(), documents.Input{Kind: documents.KindInvoice}, nil); ok {
		t.Error("an invoice went to a plugin that only renders credit memos")
	}
}
