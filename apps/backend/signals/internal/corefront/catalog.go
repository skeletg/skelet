// Package corefront reads core's public storefront API for the builder's products block - the one surface of core's the suite may call
// Asked the way the storefront asks, scoped to a channel by the same header
package corefront

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

// A block's cards are the same for every recipient of a send, so they are resolved once and kept a while
// A failed lookup is kept briefly too, so a storefront that is down is asked again in seconds and not per recipient
const (
	cardsTTL    = 5 * time.Minute
	failureTTL  = 30 * time.Second
	cacheBudget = 512 // entries, after which the oldest go
)

// Catalog is an emailtemplate.CatalogReader over core's storefront GraphQL endpoint
type Catalog struct {
	endpoint      string
	http          *http.Client
	stores        emailtemplate.StoreReader
	serviceSecret string // signs the token each request carries so core's per-IP limiter doesn't count the suite as a flood - empty sends none

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	cards   []emailtemplate.ProductCard
	err     error
	expires time.Time
}

// New returns nil for an empty endpoint - the products block then renders nothing, which is how the
// builder behaves with no catalog reader wired at all
func New(endpoint string, stores emailtemplate.StoreReader, serviceSecret string) *Catalog {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}
	return &Catalog{
		endpoint: endpoint, http: &http.Client{Timeout: 10 * time.Second}, stores: stores,
		serviceSecret: strings.TrimSpace(serviceSecret), cache: map[string]cached{},
	}
}

// remembered answers from the cache, or runs fetch once and remembers what it answered
func (c *Catalog) remembered(key string, fetch func() ([]emailtemplate.ProductCard, error)) ([]emailtemplate.ProductCard, error) {
	now := time.Now()
	c.mu.Lock()
	if hit, ok := c.cache[key]; ok && now.Before(hit.expires) {
		c.mu.Unlock()
		return slices.Clone(hit.cards), hit.err
	}
	c.mu.Unlock()
	cards, err := fetch()
	entry := cached{cards: cards, err: err, expires: now.Add(cardsTTL)}
	if err != nil {
		entry.expires = now.Add(failureTTL)
	}
	c.mu.Lock()
	if len(c.cache) >= cacheBudget {
		for k, v := range c.cache { // expired first
			if !now.Before(v.expires) {
				delete(c.cache, k)
			}
		}
		for k := range c.cache { // then whatever the map offers, down to half
			if len(c.cache) < cacheBudget/2 {
				break
			}
			delete(c.cache, k)
		}
	}
	c.cache[key] = entry
	c.mu.Unlock()
	return slices.Clone(cards), err
}

// The card fields the block renders, as the storefront API names them
type product struct {
	Name                string `json:"name"`
	SKU                 string `json:"sku"`
	URL                 string `json:"url"`
	Price               int64  `json:"price"`
	DefaultCurrencyCode string `json:"defaultCurrencyCode"`
	MainImage           *struct {
		URL string `json:"url"`
	} `json:"mainImage"`
}

const cardFields = `name sku url price defaultCurrencyCode mainImage { url }`

func (c *Catalog) ProductCardsBySKUs(ctx context.Context, channelID uuid.UUID, skus []string) ([]emailtemplate.ProductCard, error) {
	sorted := slices.Clone(skus)
	slices.Sort(sorted)
	return c.remembered("skus:"+channelID.String()+":"+strings.Join(sorted, ","), func() ([]emailtemplate.ProductCard, error) {
		var out struct {
			ProductsBySkus []product `json:"productsBySkus"`
		}
		if err := c.query(ctx, channelID, `query($skus: [String!]!) { productsBySkus(skus: $skus) { `+cardFields+` } }`,
			map[string]any{"skus": skus}, &out); err != nil {
			return nil, err
		}
		return c.cards(ctx, channelID, out.ProductsBySkus), nil
	})
}

func (c *Catalog) ProductCardsInCollection(ctx context.Context, channelID uuid.UUID, collectionID string, limit int) ([]emailtemplate.ProductCard, error) {
	return c.remembered("collection:"+channelID.String()+":"+collectionID+":"+strconv.Itoa(limit), func() ([]emailtemplate.ProductCard, error) {
		var out struct {
			Collection struct {
				Products []product `json:"products"`
			} `json:"collection"`
		}
		if err := c.query(ctx, channelID, `query($id: ID!) { collection(id: $id) { products { `+cardFields+` } } }`,
			map[string]any{"id": collectionID}, &out); err != nil {
			return nil, err
		}
		prods := out.Collection.Products
		if limit > 0 && len(prods) > limit {
			prods = prods[:limit]
		}
		return c.cards(ctx, channelID, prods), nil
	})
}

// The storefront answers paths relative to the store, so the store's public URL makes them absolute -
// the one the roster carries, since a mail client has no base to resolve against
func (c *Catalog) cards(ctx context.Context, channelID uuid.UUID, prods []product) []emailtemplate.ProductCard {
	base := ""
	if c.stores != nil {
		if st, err := c.stores.Get(ctx, channelID); err == nil {
			base = strings.TrimRight(strings.TrimSpace(st.PublicURL), "/")
		}
	}
	out := make([]emailtemplate.ProductCard, 0, len(prods))
	for _, p := range prods {
		card := emailtemplate.ProductCard{Name: p.Name}
		if card.Name == "" {
			card.Name = p.SKU
		}
		if p.Price > 0 {
			card.Price = fmt.Sprintf("%.2f %s", float64(p.Price)/100, p.DefaultCurrencyCode)
		}
		if p.MainImage != nil {
			card.Image = p.MainImage.URL
		}
		switch {
		case strings.HasPrefix(p.URL, "http://"), strings.HasPrefix(p.URL, "https://"):
			card.URL = p.URL
		case p.URL != "" && base != "":
			card.URL = base + "/" + strings.TrimPrefix(p.URL, "/")
		}
		out = append(out, card)
	}
	return out
}

func (c *Catalog) query(ctx context.Context, channelID uuid.UUID, q string, vars map[string]any, into any) error {
	body, err := json.Marshal(map[string]any{"query": q, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Channel-Id", channelID.String())
	if c.serviceSecret != "" {
		// Minted per request - a leaked one is worth two minutes of catalog reads
		tok, err := auth.GenerateServiceToken(auth.ServiceClaims{System: true}, signalsapi.CoreAudience, c.serviceSecret, 2*time.Minute)
		if err != nil {
			return err
		}
		req.Header.Set(signalsapi.ServiceTokenHeader, tok)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("storefront api answered %d", res.StatusCode)
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		return errors.New(envelope.Errors[0].Message)
	}
	return json.Unmarshal(envelope.Data, into)
}
