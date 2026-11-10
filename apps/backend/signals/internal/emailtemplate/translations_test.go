package emailtemplate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A shopper gets their language's version, a regional locale reads its language's, a translation without a subject
// keeps the email's, and a locale nobody wrote for gets the email as written
func TestLocalizePicksTheShoppersVersion(t *testing.T) {
	id := uuid.New()
	email := LibraryEmail{ID: id, Name: "Order confirmed", Subject: "Thanks for your order", Blocks: []Block{{Type: "text", Content: "Hello"}}}
	german := Translation{EmailID: id, Locale: "de", Subject: "Danke für Ihre Bestellung", Blocks: []Block{{Type: "text", Content: "Hallo"}}}
	french := Translation{EmailID: id, Locale: "fr-ca", Blocks: []Block{{Type: "text", Content: "Bonjour"}}}
	svc := NewService(fakeRepo{translations: map[uuid.UUID][]Translation{id: {german, french}}}, nil)
	ctx := context.Background()

	cases := []struct {
		locale, subject, text string
		translated            bool
	}{
		{"de", "Danke für Ihre Bestellung", "Hallo", true},
		{"DE-AT", "Danke für Ihre Bestellung", "Hallo", true},
		{"fr-ca", "Thanks for your order", "Bonjour", true},
		{"fr", "Thanks for your order", "Hello", false},
		{"", "Thanks for your order", "Hello", false},
	}
	for _, tc := range cases {
		got, translated := svc.Localize(ctx, email, tc.locale)
		if got.Subject != tc.subject || got.Blocks[0].Content != tc.text || translated != tc.translated {
			t.Errorf("%q gets %q / %q (translated %v), want %q / %q (%v)", tc.locale, got.Subject, got.Blocks[0].Content, translated, tc.subject, tc.text, tc.translated)
		}
		if got.Name != email.Name || got.ID != id {
			t.Errorf("%q changed the email's own fields: %+v", tc.locale, got)
		}
	}
	for _, bad := range []string{"  ", "german", "de_AT", "de;drop", strings.Repeat("a", 40)} {
		if _, err := svc.SaveTranslation(ctx, Translation{EmailID: id, Locale: bad, Blocks: german.Blocks}); !errors.Is(err, ErrTranslationLocale) {
			t.Errorf("a translation for %q = %v, want %v", bad, err, ErrTranslationLocale)
		}
	}
	if _, err := svc.SaveTranslation(ctx, Translation{EmailID: id, Locale: "de"}); !errors.Is(err, ErrTranslationEmpty) {
		t.Errorf("a translation with no blocks = %v, want %v", err, ErrTranslationEmpty)
	}
	if saved, err := svc.SaveTranslation(ctx, Translation{EmailID: id, Locale: " DE-AT ", Blocks: german.Blocks}); err != nil || saved.Locale != "de-at" {
		t.Errorf("a regional translation saved as %q, %v - want de-at", saved.Locale, err)
	}
}

// A language's version says the order blocks' words in its language, and a tax-inclusive order shows its tax under the
// total rather than as one more amount - a label key the blocks don't use is refused
func TestOrderBlocksSpeakTheVersionsLanguage(t *testing.T) {
	svc := NewService(fakeRepo{}, nil)
	german := LibraryEmail{Subject: "Bestellung {{order.number}}", Blocks: []Block{
		{Type: "order_items", Labels: map[string]string{"item": "Artikel", "qty": "Menge", "total": "Summe"}},
		{Type: "order_totals", Labels: map[string]string{"subtotal": "Zwischensumme", "total": "Gesamt", "taxIncluded": "inkl. MwSt."}},
		{Type: "shipping_address", Labels: map[string]string{"shippingTo": "Lieferadresse"}},
	}}
	data := sampleConfirmation()
	data.TaxIncluded, data.Total = true, 2500
	_, html, text, err := svc.RenderStoredOrderConfirmation(context.Background(), uuid.Nil, german, data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"<th>Artikel</th>", "<th>Menge</th>", "Zwischensumme", "Gesamt", "inkl. MwSt.", "Lieferadresse", "Shipping</td>"} {
		if !strings.Contains(html, want) {
			t.Errorf("html lacks %q", want)
		}
	}
	if strings.Contains(html, ">Tax<") {
		t.Error("an inclusive order still adds a Tax row above the total")
	}
	if !strings.Contains(text, "inkl. MwSt.: ") || strings.Contains(text, "\nTax: ") {
		t.Errorf("text totals:\n%s", text)
	}
	if err := svc.ValidateContent([]Block{{Type: "order_items", Labels: map[string]string{"colour": "Farbe"}}}, EmailStyles{}); err == nil {
		t.Error("an unknown label key saved")
	}
}
