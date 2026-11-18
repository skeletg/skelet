package sdk

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Which document a renderer makes
type DocumentKind string

const (
	DocumentInvoice     DocumentKind = "invoice"
	DocumentPackingSlip DocumentKind = "packing_slip"
	DocumentCreditMemo  DocumentKind = "credit_memo"
)

type DocumentAddress struct {
	FirstName, LastName, Company string
	Street1, Street2             string
	City, State, PostalCode      string
	CountryCode, Phone           string
}

type DocumentLine struct {
	SKU            string
	Name           string
	Quantity       float64
	UnitPriceCents int64
	TotalCents     int64
	Options        []string // "Size: M", "Engraving: Ada"
}

type DocumentField struct {
	Label, Value string
}

type DocumentOrder struct {
	ID           uuid.UUID
	Number       string
	PlacedAt     time.Time
	Email        string
	CurrencyCode string
	Decimals     int // the currency's minor-unit digits, so a yen total isn't divided by 100
	TaxIncluded  bool
	Billing      DocumentAddress
	Shipping     DocumentAddress
	BusinessIDs  []DocumentField
	PONumber     string
	Lines        []DocumentLine
	Fees         []DocumentFee // what plugins charged on top, in TotalCents

	SubtotalCents, DiscountCents, ShippingCents, TaxCents, TotalCents int64
	GiftCardCents, StoreCreditCents                                   int64
}

type DocumentFee struct {
	Label string
	Cents int64
}

type DocumentMemo struct {
	ID              uuid.UUID
	Number          string
	CreatedAt       time.Time
	Lines           []DocumentLine
	ShippingCents   int64
	FeesCents       int64 // refunded of the fees plugins charged - FeeCents below is the restocking fee taken off
	AdjustmentCents int64
	FeeCents        int64
	TotalCents      int64
	Reason          string
}

// The channel's document settings, everything but the logo as it read when the order was placed. Logo is a PNG at
// most 800 pixels wide, and Note the one set on the buyer's country, for a business purchase's invoices and credit memos
type DocumentSeller struct {
	Name    string
	Details []string
	Logo    []byte
	Footer  string
	Note    string
}

// Default is core's own PDF, for a renderer that only stamps or wraps it
type DocumentRequest struct {
	Kind      DocumentKind
	ChannelID uuid.UUID
	Order     DocumentOrder
	Memo      *DocumentMemo // a credit memo's, nil otherwise
	Seller    DocumentSeller
	Default   func() ([]byte, error)
}

// Returns the PDF - anything else is refused, and an error reaches the admin who asked for the document
type DocumentRenderer func(ctx context.Context, req DocumentRequest) ([]byte, error)

type RegisteredDocumentRenderer struct {
	PluginSlug string
	Kind       DocumentKind
	Render     DocumentRenderer
}

var documentRenderers registry[RegisteredDocumentRenderer]

// Makes the kind's documents on every channel the plugin is enabled for - panics on an unknown kind, a nil renderer or a
// second one for the same kind from the same plugin
func (pl *Plugin) RegisterDocumentRenderer(kind DocumentKind, r DocumentRenderer) {
	switch {
	case !slices.Contains([]DocumentKind{DocumentInvoice, DocumentPackingSlip, DocumentCreditMemo}, kind):
		panic("sdk.RegisterDocumentRenderer: unknown document kind " + string(kind))
	case r == nil:
		panic("sdk.RegisterDocumentRenderer: nil renderer for " + string(kind))
	}
	documentRenderers.add(RegisteredDocumentRenderer{PluginSlug: pl.slug, Kind: kind, Render: r},
		func(d RegisteredDocumentRenderer) bool { return d.PluginSlug == pl.slug && d.Kind == kind },
		"sdk.RegisterDocumentRenderer: "+pl.slug+" already renders "+string(kind))
}

func DocumentRenderers() []RegisteredDocumentRenderer { return documentRenderers.all() }
