package shoptools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

const storeDoc = `query AgentStore {
  currentChannel { name currencyCode defaultLocale locales { code name } }
  countries(activeOnly: true) { code name }
  productSortOptions { slug label }
  freeShippingThreshold
}`

func storeInfoTool() tool {
	return tool{
		def: ai.Tool{
			Name:        "get_store_info",
			Description: "The store's basics: name, currency, languages, the countries it delivers to, how search results can be sorted, and when shipping is free.",
			Schema:      schema(map[string]any{}),
			ReadOnly:    true, Idempotent: true,
		},
		run: runStoreInfo,
	}
}

func runStoreInfo(ctx context.Context, c call) (string, error) {
	var out struct {
		CurrentChannel *struct {
			Name          string `json:"name"`
			CurrencyCode  string `json:"currencyCode"`
			DefaultLocale string `json:"defaultLocale"`
			Locales       []struct {
				Code string `json:"code"`
				Name string `json:"name"`
			} `json:"locales"`
		} `json:"currentChannel"`
		Countries []struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"countries"`
		ProductSortOptions []struct {
			Slug  string `json:"slug"`
			Label string `json:"label"`
		} `json:"productSortOptions"`
		FreeShippingThreshold int64 `json:"freeShippingThreshold"`
	}
	if err := c.deps.GraphQL.Do(ctx, storeDoc, nil, &out); err != nil {
		return "", err
	}
	var b strings.Builder
	name, cur := c.ch.Name, c.ch.CurrencyCode
	if ch := out.CurrentChannel; ch != nil {
		name, cur = ch.Name, ch.CurrencyCode
	}
	fmt.Fprintf(&b, "%s - %s\nPrices in %s.\n", name, link(c.ch, "/"), cur)
	if ch := out.CurrentChannel; ch != nil && len(ch.Locales) > 1 {
		langs := make([]string, len(ch.Locales))
		for i, l := range ch.Locales {
			langs[i] = fmt.Sprintf("%s (%s)", l.Name, l.Code)
		}
		fmt.Fprintf(&b, "Languages: %s\n", strings.Join(langs, ", "))
	}
	switch n := len(out.Countries); {
	case n == 0:
	case n <= 40:
		names := make([]string, n)
		for i, co := range out.Countries {
			names[i] = fmt.Sprintf("%s (%s)", co.Name, co.Code)
		}
		fmt.Fprintf(&b, "Sells to: %s - estimate_shipping says what delivery each one gets.\n", strings.Join(names, ", "))
	default:
		fmt.Fprintf(&b, "Sells to %d countries - estimate_shipping takes a country code and says what delivery it gets.\n", n)
	}
	if out.FreeShippingThreshold > 0 {
		fmt.Fprintf(&b, "Free shipping from %s, depending on the country.\n", price(out.FreeShippingThreshold, cur))
	}
	if len(out.ProductSortOptions) > 0 {
		sorts := make([]string, len(out.ProductSortOptions))
		for i, s := range out.ProductSortOptions {
			sorts[i] = fmt.Sprintf("%s (%s)", s.Slug, s.Label)
		}
		fmt.Fprintf(&b, "Sorts for search_products: %s\n", strings.Join(sorts, ", "))
	}
	b.WriteString("Shipping, returns and other policies: search_store_pages.\n")
	return b.String(), nil
}

const pagesDoc = `query AgentPages { cmsPages(filter: {onlyPublished: true}, limit: 200) { urlPath content { title body data } } }`

func pagesTool() tool {
	return tool{
		def: ai.Tool{
			Name:        "search_store_pages",
			Description: "Search the store's own pages - shipping, returns, sizing, FAQs, contact and the like. Omit query to list every page.",
			Schema: schema(map[string]any{
				"query": str("Words to look for, like \"returns\" or \"delivery time\"."),
			}),
			ReadOnly: true, Idempotent: true,
		},
		run: runPages,
	}
}

type page struct {
	title, url, text string
	score            int
}

func runPages(ctx context.Context, c call) (string, error) {
	var out struct {
		CmsPages []struct {
			URLPath string `json:"urlPath"`
			Content *struct {
				Title string          `json:"title"`
				Body  json.RawMessage `json:"body"`
				Data  json.RawMessage `json:"data"`
			} `json:"content"`
		} `json:"cmsPages"`
	}
	if err := c.deps.GraphQL.Do(ctx, pagesDoc, nil, &out); err != nil {
		return "", err
	}
	words := strings.Fields(strings.ToLower(c.a.Query))
	var hits []page
	for _, p := range out.CmsPages {
		if p.Content == nil || p.Content.Title == "" {
			continue
		}
		text := strings.TrimSpace(plainText(p.Content.Body, 4000) + " " + plainText(p.Content.Data, 4000))
		pg := page{title: p.Content.Title, url: link(c.ch, p.URLPath), text: text}
		title, body := strings.ToLower(pg.title+" "+p.URLPath), strings.ToLower(text)
		matched := true
		for _, w := range words {
			switch {
			case strings.Contains(title, w):
				pg.score += 3
			case strings.Contains(body, w):
				pg.score++
			default:
				matched = false
			}
		}
		if matched {
			hits = append(hits, pg)
		}
	}
	if len(hits) == 0 {
		if len(words) > 0 {
			return "No store page mentions that. Omit query to see every page.", nil
		}
		return "The store has no pages published.", nil
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	var b strings.Builder
	if len(words) == 0 {
		b.WriteString("Store pages:\n")
		for _, p := range hits[:min(len(hits), 50)] {
			fmt.Fprintf(&b, "- %s - %s\n", p.title, p.url)
		}
		return b.String(), nil
	}
	for _, p := range hits[:min(len(hits), 4)] {
		fmt.Fprintf(&b, "## %s - %s\n%s\n\n", p.title, p.url, clip(p.text, 1500))
	}
	return b.String(), nil
}

const shippingDoc = `query AgentShipping($country: String!, $cartId: ID, $region: String, $postal: String) {
  shippingQuotes(countryCode: $country, cartId: $cartId, regionCode: $region, postalCode: $postal) { carrierName name priceCents estimatedDays estimatedArrival }
  freeShippingThreshold(countryCode: $country)
}`

func shippingTool() tool {
	return tool{
		def: ai.Tool{
			Name:        "estimate_shipping",
			Description: "The delivery options to a country with their prices and times. Pass cart_id to price them for that cart, as checkout will.",
			Schema: schema(map[string]any{
				"country":     str("ISO 3166-1 alpha-2 country code, like \"DE\" or \"US\"."),
				"region":      str("Region or state code, where the country has them."),
				"postal_code": str("Postal code, for options that depend on it."),
				"cart_id":     str("A cart id from update_cart."),
			}, "country"),
			ReadOnly: true, Idempotent: true,
		},
		run: runShipping,
	}
}

func runShipping(ctx context.Context, c call) (string, error) {
	country := strings.ToUpper(strings.TrimSpace(c.a.Country))
	if len(country) != 2 {
		return "", refusal("country takes a two-letter ISO code, like \"DE\".")
	}
	vars := map[string]any{"country": country, "region": nilIfEmpty(c.a.Region), "postal": nilIfEmpty(c.a.PostalCode), "cartId": nil}
	var subtotal int64 = -1
	if c.a.CartID != "" {
		ct, err := agentCart(ctx, c, c.a.CartID)
		if err != nil {
			return "", err
		}
		vars["cartId"] = ct.ID.String()
		subtotal = 0
		for _, l := range ct.Lines {
			subtotal += int64(float64(l.UnitPrice)*l.Quantity + 0.5)
		}
	}
	var out struct {
		ShippingQuotes []struct {
			CarrierName      string  `json:"carrierName"`
			Name             string  `json:"name"`
			PriceCents       int64   `json:"priceCents"`
			EstimatedDays    *int    `json:"estimatedDays"`
			EstimatedArrival *string `json:"estimatedArrival"`
		} `json:"shippingQuotes"`
		FreeShippingThreshold int64 `json:"freeShippingThreshold"`
	}
	if err := c.deps.GraphQL.Do(ctx, shippingDoc, vars, &out); err != nil {
		return "", err
	}
	cur := c.ch.CurrencyCode
	if len(out.ShippingQuotes) == 0 {
		return fmt.Sprintf("The store doesn't deliver to %s.", country), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Delivery to %s:\n", country)
	for _, q := range out.ShippingQuotes {
		cost := price(q.PriceCents, cur)
		if q.PriceCents == 0 {
			cost = "free"
		}
		fmt.Fprintf(&b, "- %s", q.Name)
		if q.CarrierName != "" && q.CarrierName != q.Name {
			fmt.Fprintf(&b, " (%s)", q.CarrierName)
		}
		fmt.Fprintf(&b, ": %s", cost)
		switch {
		case q.EstimatedArrival != nil && *q.EstimatedArrival != "":
			fmt.Fprintf(&b, ", arrives around %s", *q.EstimatedArrival)
		case q.EstimatedDays != nil:
			fmt.Fprintf(&b, ", about %d days", *q.EstimatedDays)
		}
		b.WriteString("\n")
	}
	if t := out.FreeShippingThreshold; t > 0 {
		if subtotal >= 0 && subtotal < t {
			fmt.Fprintf(&b, "Free shipping from %s - the cart is %s short.\n", price(t, cur), price(t-subtotal, cur))
		} else if subtotal < 0 {
			fmt.Fprintf(&b, "Free shipping from %s.\n", price(t, cur))
		}
	}
	b.WriteString("Tax and the final shipping price are settled at checkout.\n")
	return b.String(), nil
}

func nilIfEmpty(s string) any {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return s
}
