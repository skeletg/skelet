package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/locale"
)

// A store whose channel carries en (default) and an ACTIVE de
func (h *harness) bilingualStore(t *testing.T, stock float64, priceCents int64) *store {
	t.Helper()
	h.ensureLocale(t, "de")
	return h.newStoreOpt(t, stock, priceCents, func(in *channel.CreateChannelInput) {
		in.DefaultLocale = "en"
		in.Locales = []string{"en", "de"}
	})
}

// Scenario: translations live per (product, locale) - a hit serves the translation, a miss falls back to the default
// locale, writes validate the locale, and the default row is undeletable while translations remain
func TestProductContentLocalizesAndFallsBack(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 1000)
	if _, err := h.d.Catalog.UpsertProductContent(h.ctx, f.Product.ID, catalog.ProductContentInput{
		Locale: "xx", Name: "nope",
	}); !errors.Is(err, locale.ErrLocaleNotFound) {
		t.Errorf("unregistered locale write = %v, want ErrLocaleNotFound", err)
	}

	h.ensureLocale(t, "de")
	if _, err := h.d.Catalog.UpsertProductContent(h.ctx, f.Product.ID, catalog.ProductContentInput{
		Locale: "de", Name: "Werkzeug DE",
	}); err != nil {
		t.Fatalf("write de content: %v", err)
	}

	hit, err := h.d.Catalog.GetProduct(h.ctx, f.Product.ID, "de")
	if err != nil || hit.Content == nil {
		t.Fatalf("read de: %v (content %v)", err, hit.Content)
	}
	if hit.Content.Locale != "de" || hit.Content.Name != "Werkzeug DE" {
		t.Errorf("de read = %s/%q, want the translation", hit.Content.Locale, hit.Content.Name)
	}
	if len(hit.AllContent) != 2 {
		t.Errorf("AllContent = %d rows, want en + de", len(hit.AllContent))
	}

	// A product with no de row serves the default locale's content instead
	other := h.addProduct(t, f, 700, true, 5)
	miss, err := h.d.Catalog.GetProduct(h.ctx, other.ID, "de")
	if err != nil || miss.Content == nil {
		t.Fatalf("fallback read: %v (content %v)", err, miss.Content)
	}
	if miss.Content.Locale != "en" {
		t.Errorf("fallback locale = %s, want en", miss.Content.Locale)
	}

	// The default-locale row anchors the fallbacks - it refuses to die first
	if err := h.d.Catalog.DeleteProductContent(h.ctx, f.Product.ID, "en"); !errors.Is(err, catalog.ErrCannotDeleteDefaultLocale) {
		t.Errorf("default-delete error = %v, want ErrCannotDeleteDefaultLocale", err)
	}

	// And products are only ever BORN in the default locale
	if _, err := h.d.Catalog.CreateProduct(h.ctx, catalog.CreateProductInput{
		ProductTypeID: f.Product.ProductTypeID, Locale: "de",
		Name: "Geboren", DefaultCurrencyCode: "USD",
	}); !errors.Is(err, catalog.ErrLocaleMustBeDefault) {
		t.Errorf("non-default create = %v, want ErrLocaleMustBeDefault", err)
	}
}

// gqlPostLocale is gqlPost plus the Locale header the storefront sends
func (h *harness) gqlPostLocale(t *testing.T, ts *httptest.Server, channelID, loc, query string) (map[string]any, []string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/front/graphql", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Channel-Id", channelID)
	if loc != "" {
		req.Header.Set("Locale", loc)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("post graphql: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var msgs []string
	for _, e := range out.Errors {
		msgs = append(msgs, e.Message)
	}
	return out.Data, msgs
}

// Scenario: the wire speaks the clamped locale - a supported header serves the translation (case- insensitively), an unsupported one snaps
// to the channel default, and a channel that doesn't carry the locale never serves it however translated the product is
func TestLocaleHeaderClampsOverHTTP(t *testing.T) {
	h := newHarness(t)
	f := h.bilingualStore(t, 5, 1500)
	if _, err := h.d.Catalog.UpsertProductContent(h.ctx, f.Product.ID, catalog.ProductContentInput{
		Locale: "de", Name: "Werkzeug DE",
	}); err != nil {
		t.Fatalf("write de content: %v", err)
	}
	en, err := h.d.Catalog.GetProduct(h.ctx, f.Product.ID, "en")
	if err != nil || en.Content == nil {
		t.Fatalf("read en content: %v", err)
	}
	englishName := en.Content.Name

	ts := httptest.NewServer(h.d.Root.Handler)
	defer ts.Close()
	query := fmt.Sprintf(`{ product(id: %q) { name } }`, f.Product.ID)

	name := func(loc string) string {
		t.Helper()
		data, errs := h.gqlPostLocale(t, ts, f.Channel.ID.String(), loc, query)
		if len(errs) > 0 {
			t.Fatalf("product query (Locale %q): %v", loc, errs)
		}
		return gqlField(t, data, "product", "name")
	}

	if got := name("de"); got != "Werkzeug DE" {
		t.Errorf("Locale de = %q, want the translation", got)
	}
	if got := name("DE"); got != "Werkzeug DE" {
		t.Errorf("Locale DE = %q, want case-insensitive matching", got)
	}
	if got := name("fr"); got != englishName {
		t.Errorf("unsupported Locale fr = %q, want the clamp to the channel default %q", got, englishName)
	}
	if got := name(""); got != englishName {
		t.Errorf("no Locale header = %q, want the channel default %q", got, englishName)
	}

	// A monolingual store never serves de, translation or not
	mono := h.newStore(t, 5, 1200)
	if _, err := h.d.Catalog.UpsertProductContent(h.ctx, mono.Product.ID, catalog.ProductContentInput{
		Locale: "de", Name: "Unsichtbar DE",
	}); err != nil {
		t.Fatalf("write de on mono product: %v", err)
	}
	data, errs := h.gqlPostLocale(t, ts, mono.Channel.ID.String(), "de",
		fmt.Sprintf(`{ product(id: %q) { name } }`, mono.Product.ID))
	if len(errs) > 0 {
		t.Fatalf("mono query: %v", errs)
	}
	if got := gqlField(t, data, "product", "name"); strings.Contains(got, "Unsichtbar") {
		t.Errorf("monolingual channel served %q - the clamp must snap de to en", got)
	}
}
