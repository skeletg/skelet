package email

import (
	"bytes"
	"fmt"
	"html/template"
	ttemplate "text/template"
)

// An offer cart an admin built and sent - the lines as the recipient will see them, and the link that opens them
type CartOfferEmailData struct {
	FirstName string
	StoreName string
	Link      string
	Message   string // the admin's note, empty when none
	ExpiresAt string // already formatted for the reader
	Lines     []CartOfferLine
	Total     string // the lines' subtotal, formatted - tax and delivery come at checkout
}

type CartOfferLine struct {
	Name     string
	Quantity string
	Total    string
}

const subjectCartOffer = "%s prepared an offer for you"

var cartOfferHTML = template.Must(template.New("cart_offer_html").Parse(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;color:#222;">
<p>Hi {{.FirstName}},</p>
<p>{{.StoreName}} has put together an order for you.</p>
{{if .Message}}<p style="white-space:pre-line;">{{.Message}}</p>{{end}}
<table style="border-collapse:collapse;margin:12px 0;">
{{range .Lines}}<tr><td style="padding:4px 12px 4px 0;">{{.Name}}</td><td style="padding:4px 12px;">× {{.Quantity}}</td><td style="padding:4px 0;text-align:right;">{{.Total}}</td></tr>
{{end}}<tr><td colspan="2" style="padding:8px 12px 4px 0;font-weight:bold;">Subtotal</td><td style="padding:8px 0 4px;text-align:right;font-weight:bold;">{{.Total}}</td></tr>
</table>
<p><a href="{{.Link}}">View your offer</a></p>
<p>The offer is open until {{.ExpiresAt}}. Tax and delivery are added at checkout.</p>
</body></html>`))

var cartOfferText = ttemplate.Must(ttemplate.New("cart_offer_text").Parse(
	`Hi {{.FirstName}},

{{.StoreName}} has put together an order for you.
{{if .Message}}
{{.Message}}
{{end}}
{{range .Lines}}- {{.Name}} × {{.Quantity}}: {{.Total}}
{{end}}Subtotal: {{.Total}}

View your offer: {{.Link}}

The offer is open until {{.ExpiresAt}}. Tax and delivery are added at checkout.
`))

func RenderCartOfferEmail(data CartOfferEmailData) (subject, html, text string, err error) {
	if data.FirstName == "" {
		data.FirstName = greetingFallback
	}
	var hb, tb bytes.Buffer
	if err := cartOfferHTML.Execute(&hb, data); err != nil {
		return "", "", "", fmt.Errorf("render html: %w", err)
	}
	if err := cartOfferText.Execute(&tb, data); err != nil {
		return "", "", "", fmt.Errorf("render text: %w", err)
	}
	return fmt.Sprintf(subjectCartOffer, data.StoreName), hb.String(), tb.String(), nil
}

// FormatMoney renders minor units the way every email body shows an amount - "12.34 USD"
func FormatMoney(minor int64, currency string) string { return formatMoney(minor, currency) }
