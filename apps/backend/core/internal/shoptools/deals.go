package shoptools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

var dealsDoc = document("deals", `query AgentDeals($limit: Int) {
  storePromotions { label terms endsAt }
  saleProducts(limit: $limit) { id sku name kind url price compareAtPrice defaultCurrencyCode inStock }
}`)

func dealsTool() tool {
	return tool{
		def: ai.Tool{
			Name: "get_deals",
			Description: "What's on offer right now: the store's automatic promotions with their terms, and the products selling below their usual price, " +
				"the biggest saving first. One product's own promotions come with get_product.",
			Schema: schema(map[string]any{
				"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": catalog.MaxSalePage, "description": fmt.Sprintf("Sale products to list (default %d).", catalog.SalePageSize)},
			}),
			Effect: ai.Reads, Idempotent: true,
		},
		run: runDeals,
	}
}

func runDeals(ctx context.Context, c call) (string, error) {
	limit := clamp(c.a.Limit, catalog.SalePageSize, catalog.MaxSalePage)
	var out struct {
		StorePromotions []struct {
			Label  string     `json:"label"`
			Terms  string     `json:"terms"`
			EndsAt *time.Time `json:"endsAt"`
		} `json:"storePromotions"`
		SaleProducts []productHit `json:"saleProducts"`
	}
	if err := c.gql().Do(ctx, dealsDoc, map[string]any{"limit": limit}, &out); err != nil {
		return "", err
	}
	var b strings.Builder
	if len(out.StorePromotions) == 0 {
		b.WriteString("No automatic promotions are running right now.\n")
	} else {
		b.WriteString("Promotions running now, applied in the cart without a code once their terms are met:\n")
	}
	for _, p := range out.StorePromotions {
		fmt.Fprintf(&b, "- %s", p.Label)
		if terms := clip(p.Terms, summaryChars); terms != "" {
			fmt.Fprintf(&b, ": %s", terms)
		}
		if p.EndsAt != nil {
			fmt.Fprintf(&b, " (until %s)", p.EndsAt.Format("2 Jan 2006"))
		}
		b.WriteString("\n")
	}
	if len(out.SaleProducts) == 0 {
		b.WriteString("Nothing is marked down right now.\n")
		return b.String(), nil
	}
	b.WriteString("On sale now, the biggest saving first:\n")
	for _, p := range out.SaleProducts {
		b.WriteString("- ")
		writeHit(&b, c, p)
	}
	return b.String(), nil
}
