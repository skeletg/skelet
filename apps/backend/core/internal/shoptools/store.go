package shoptools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
)

const (
	countriesListed  = 40
	pagesPerRequest  = 100 // the front resolver's own ceiling
	maxPagesScanned  = 500
	pageTextChars    = 3000
	pageExcerptChars = 1500
	pagesShown       = 4
	pageListShown    = 50
	pageCacheTTL     = time.Minute
)

var storeDoc = document("store", `query AgentStore {
  currentChannel { name currencyCode defaultLocale locales { code name } }
  countries(activeOnly: true) { code name }
  productSortOptions { slug label }
  freeShippingThreshold
}`)

func storeInfoTool() tool {
	return tool{
		def: ai.Tool{
			Name:        "get_store_info",
			Description: "The store's basics: name, currency, languages, the countries it sells to, how search results can be sorted, and when shipping is free.",
			Schema:      schema(map[string]any{}),
			Effect:      ai.Reads, Idempotent: true,
		},
		run: runStoreInfo,
	}
}

func runStoreInfo(ctx context.Context, c call) (string, error) {
	var out struct {
		CurrentChannel *struct {
			Name         string `json:"name"`
			CurrencyCode string `json:"currencyCode"`
			Locales      []struct {
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
	if err := c.gql().Do(ctx, storeDoc, nil, &out); err != nil {
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
	case n <= countriesListed:
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

var pagesDoc = document("pages", `query AgentPages($limit: Int, $offset: Int) {
  cmsPages(filter: {onlyPublished: true}, limit: $limit, offset: $offset) { urlPath content { title body data } }
}`)

func pagesTool() tool {
	return tool{
		def: ai.Tool{
			Name:        "search_store_pages",
			Description: "Search the store's own pages - shipping, returns, sizing, FAQs, contact and the like. Omit query to list every page.",
			Schema: schema(map[string]any{
				"query": str("Words to look for, like \"returns\" or \"delivery time\"."),
			}),
			Effect: ai.Reads, Idempotent: true,
		},
		run: runPages,
	}
}

type page struct {
	title, url, text  string
	titleKey, textKey string
}

type pageEntry struct {
	at    time.Time
	pages []page
}

// pageCache keeps each channel's page text for a minute per language - every search would otherwise load and parse every page's blocks
type pageCache struct {
	mu      sync.Mutex
	entries map[string]pageEntry
}

func (pc *pageCache) get(key string) ([]page, bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	e, ok := pc.entries[key]
	return e.pages, ok && time.Since(e.at) < pageCacheTTL
}

func (pc *pageCache) put(key string, pages []page) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.entries[key] = pageEntry{at: time.Now(), pages: pages}
}

func storePages(ctx context.Context, c call) ([]page, error) {
	key := c.ch.ID.String() + "|" + locale.FromContext(ctx)
	if pages, ok := c.svc.pages.get(key); ok {
		return pages, nil
	}
	var pages []page
	for offset := 0; offset < maxPagesScanned; offset += pagesPerRequest {
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
		if err := c.gql().Do(ctx, pagesDoc, map[string]any{"limit": pagesPerRequest, "offset": offset}, &out); err != nil {
			return nil, err
		}
		for _, p := range out.CmsPages {
			if p.Content == nil || p.Content.Title == "" {
				continue
			}
			text := strings.TrimSpace(plainText(p.Content.Body, pageTextChars) + " " + plainText(p.Content.Data, pageTextChars))
			pages = append(pages, page{title: p.Content.Title, url: link(c.ch, p.URLPath), text: text,
				titleKey: strings.ToLower(p.Content.Title + " " + p.URLPath), textKey: strings.ToLower(text)})
		}
		if len(out.CmsPages) < pagesPerRequest {
			break
		}
	}
	c.svc.pages.put(key, pages)
	return pages, nil
}

func runPages(ctx context.Context, c call) (string, error) {
	pages, err := storePages(ctx, c)
	if err != nil {
		return "", err
	}
	if len(pages) == 0 {
		return "The store has no pages published.", nil
	}
	words := strings.Fields(strings.ToLower(c.a.Query))
	var b strings.Builder
	if len(words) == 0 {
		b.WriteString("Store pages:\n")
		for _, p := range pages[:min(len(pages), pageListShown)] {
			fmt.Fprintf(&b, "- %s - %s\n", p.title, p.url)
		}
		return b.String(), nil
	}
	type hit struct {
		page
		score int
	}
	var hits []hit
	for _, p := range pages {
		if score, ok := pageScore(p, words); ok {
			hits = append(hits, hit{p, score})
		}
	}
	if len(hits) == 0 {
		return "No store page mentions that. Omit query to see every page.", nil
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	for _, h := range hits[:min(len(hits), pagesShown)] {
		fmt.Fprintf(&b, "## %s - %s\n%s\n\n", h.title, h.url, clip(h.text, pageExcerptChars))
	}
	return b.String(), nil
}

func pageScore(p page, words []string) (int, bool) {
	score := 0
	for _, w := range words {
		switch {
		case strings.Contains(p.titleKey, w):
			score += 3
		case strings.Contains(p.textKey, w):
			score++
		default:
			return 0, false
		}
	}
	return score, true
}

var (
	shippingDoc = document("shipping", `query AgentShipping($country: String!, $cartId: ID, $region: String, $postal: String) {
  shippingQuotes(countryCode: $country, cartId: $cartId, regionCode: $region, postalCode: $postal) { carrierName name priceCents estimatedDays estimatedArrival }
  freeShippingThreshold(countryCode: $country)
}`)
	cartTotalsDoc = document("cartTotals", `query AgentCartTotals($id: ID!) { cart(id: $id) { subtotal totalDiscount } }`)
)

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
			Effect: ai.Reads, Idempotent: true,
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
	spent := int64(-1)
	if c.a.CartID != "" {
		ct, err := agentCart(ctx, c, c.a.CartID)
		if err != nil {
			return "", err
		}
		vars["cartId"] = ct.ID.String()
		var totals struct {
			Cart *struct {
				Subtotal      int64 `json:"subtotal"`
				TotalDiscount int64 `json:"totalDiscount"`
			} `json:"cart"`
		}
		if err := c.gql().Do(ctx, cartTotalsDoc, map[string]any{"id": ct.ID.String()}, &totals); err != nil {
			return "", err
		}
		if totals.Cart != nil {
			spent = max(totals.Cart.Subtotal-totals.Cart.TotalDiscount, 0)
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
	if err := c.gql().Do(ctx, shippingDoc, vars, &out); err != nil {
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
	switch t := out.FreeShippingThreshold; {
	case t <= 0:
	case spent < 0:
		fmt.Fprintf(&b, "Free shipping from %s.\n", price(t, cur))
	case spent < t:
		fmt.Fprintf(&b, "Free shipping from %s - the cart is %s short.\n", price(t, cur), price(t-spent, cur))
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
