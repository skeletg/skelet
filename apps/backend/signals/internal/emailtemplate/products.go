package emailtemplate

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/google/uuid"
)

// One resolved catalog product for the "products" email block - fully formed by the catalog reader, with price already formatted in the channel's currency and the URL already absolute, so this package stays free of catalog and media coupling
type ProductCard struct {
	Name  string
	Image string // absolute URL, may be ""
	Price string // display string, e.g. "$29.00", may be ""
	URL   string // absolute storefront URL
}

// Resolves products for the "products" block - implemented by an adapter in main.go over the catalog service, and nil makes the block render nothing, which is graceful when the reader isn't wired
type CatalogReader interface {
	ProductCardsBySKUs(ctx context.Context, channelID uuid.UUID, skus []string) ([]ProductCard, error)
	ProductCardsInCollection(ctx context.Context, channelID uuid.UUID, collectionID string, limit int) ([]ProductCard, error)
}

func (s *Service) SetCatalogReader(r CatalogReader) { s.catalog = r }

// Resolves a products block to a row of cards - best-effort, so a reader error or no products renders nothing and the email still sends
func (s *Service) renderProducts(ctx context.Context, b Block, rc renderCtx) (string, string, error) {
	if s.catalog == nil || b.Products == nil {
		return "", "", nil
	}
	p := b.Products
	limit := p.Limit
	if limit <= 0 {
		limit = 3
	}
	var cards []ProductCard
	var err error
	switch p.Mode {
	case "collection":
		if strings.TrimSpace(p.CollectionID) == "" {
			return "", "", nil
		}
		cards, err = s.catalog.ProductCardsInCollection(ctx, rc.ChannelID, p.CollectionID, limit)
	default: // "skus"
		if len(p.SKUs) == 0 {
			return "", "", nil
		}
		cards, err = s.catalog.ProductCardsBySKUs(ctx, rc.ChannelID, p.SKUs)
	}
	if err != nil || len(cards) == 0 {
		return "", "", nil // never fail a send over a merchandising block
	}
	if len(cards) > limit {
		cards = cards[:limit]
	}

	cols := p.Columns
	if cols <= 0 {
		cols = len(cards)
	}
	if cols > 4 {
		cols = 4
	}
	width := 100 / cols

	var h, t strings.Builder
	h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:8px 0"><tr>`)
	for i, c := range cards {
		if i > 0 && i%cols == 0 {
			h.WriteString(`</tr><tr>`)
		}
		fmt.Fprintf(&h, `<td valign="top" width="%d%%" style="padding:8px;text-align:center;font-family:inherit">`, width)
		if c.URL != "" {
			fmt.Fprintf(&h, `<a href="%s" style="text-decoration:none;color:inherit">`, html.EscapeString(c.URL))
		}
		if c.Image != "" {
			fmt.Fprintf(&h, `<img src="%s" alt="%s" style="max-width:100%%;height:auto;border-radius:8px"><br>`, html.EscapeString(c.Image), html.EscapeString(c.Name))
		}
		fmt.Fprintf(&h, `<span style="display:block;margin-top:6px;font-weight:600">%s</span>`, html.EscapeString(c.Name))
		if c.Price != "" {
			fmt.Fprintf(&h, `<span style="display:block;color:#555">%s</span>`, html.EscapeString(c.Price))
		}
		if c.URL != "" {
			h.WriteString(`</a>`)
		}
		h.WriteString(`</td>`)

		t.WriteString(c.Name)
		if c.Price != "" {
			t.WriteString(" - " + c.Price)
		}
		if c.URL != "" {
			t.WriteString(" " + c.URL)
		}
		t.WriteString("\n")
	}
	h.WriteString(`</tr></table>`)
	return boxHTML(b, h.String()), t.String(), nil
}
