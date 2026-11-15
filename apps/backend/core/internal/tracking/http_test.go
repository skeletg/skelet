package tracking

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestOnlyABrowsersOwnEventsAreAccepted(t *testing.T) {
	ch := uuid.New()
	read := func(body, consent string) (Event, bool) {
		r := httptest.NewRequest(http.MethodPost, TrackPath, strings.NewReader(body))
		r.RemoteAddr = "203.0.113.7:4000"
		r.Header.Set(auth.ConsentHeader, consent)
		r.Header.Set(auth.AnonHeader, "visitor-1")
		r.Header.Set(auth.ClickIDsHeader, "gclid=g1")
		return browserEvent(httptest.NewRecorder(), r, ch, auth.CustomerVerifier{})
	}
	ev, ok := read(`{"event_id":"e-1","name":"view_item","page_url":"https://shop.example/p/mug","page_title":"Blue mug","ecommerce":{"currency":"EUR","value":15,"items":[{"item_id":"SKU-1"}]}}`, "v2:a1.d0.p0")
	if !ok || ev.Name != ViewItem || ev.Source != "browser" || ev.ChannelID != ch || ev.Ecommerce.Items[0].ItemID != "SKU-1" || ev.Context.PageTitle != "Blue mug" {
		t.Fatalf("a view_item was not read: %+v", ev)
	}
	if ev.User.AnonID != "visitor-1" || ev.ClickIDs != nil || ev.Context.IP != "" {
		t.Errorf("analytics consent alone must carry the visitor and nothing for ads: %+v %+v %v", ev.User, ev.Context, ev.ClickIDs)
	}
	for name, body := range map[string]string{
		"a purchase, which only the server reports": `{"event_id":"e-2","name":"purchase"}`,
		"a cart add, which only the server reports": `{"event_id":"e-3","name":"add_to_cart"}`,
		"a shipping choice, which the cart reports": `{"event_id":"e-6","name":"add_shipping_info"}`,
		"a sign-in, which only the server reports":  `{"event_id":"e-7","name":"login"}`,
		"a search term too long to be one":          `{"event_id":"e-8","name":"search","search_term":"` + strings.Repeat("x", 201) + `"}`,
		"an event with no id":                       `{"name":"page_view"}`,
		"an id that isn't a token":                  `{"event_id":"a b;c","name":"page_view"}`,
		"not json":                                  `nope`,
		"a page url too long to be one":             `{"event_id":"e-4","name":"page_view","page_url":"` + strings.Repeat("x", 3000) + `"}`,
	} {
		if _, ok := read(body, "v2:a1.d1.p1"); ok {
			t.Errorf("accepted %s", name)
		}
	}
	if ev, ok := read(`{"event_id":"e-5","name":"page_view"}`, ""); !ok || ev.Consent != nil || ev.User.AnonID != "" {
		t.Errorf("a request stating no consent must carry none: %+v", ev)
	}
	if ev, ok := read(`{"event_id":"e-9","name":"search","search_term":"blue mug"}`, "v2:a1.d0.p0"); !ok || ev.SearchTerm != "blue mug" {
		t.Errorf("a search lost its term: %+v", ev)
	}
	ev, ok = read(`{"event_id":"e-10","name":"select_promotion","ecommerce":{"promotion_id":"r-1","promotion_name":"summer","items":[{"item_id":"SKU-1","index":0}]}}`, "v2:a1.d0.p0")
	if !ok || ev.Ecommerce.PromotionID != "r-1" || ev.Ecommerce.Items[0].Index == nil || *ev.Ecommerce.Items[0].Index != 0 {
		t.Errorf("a promotion click lost its promotion or the item's place: %+v", ev.Ecommerce)
	}
}

// The golden bodies in sdk/testdata/events are what the storefront's trackPayload sends, one per browser event. Each must
// be accepted whole: its page, its term and every ecommerce key it carries reach the event - a key the SDK doesn't know
// would be dropped here, silently, before any destination saw it. And there is a body for every name the browser may send
func TestTheGoldenBrowserEventsAreReadWhole(t *testing.T) {
	files, err := filepath.Glob("../../../sdk/testdata/events/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden events found (%v)", err)
	}
	seen := map[string]bool{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var golden struct {
			Name       string          `json:"name"`
			PageURL    string          `json:"page_url"`
			Referrer   string          `json:"referrer"`
			Locale     string          `json:"locale"`
			PageTitle  string          `json:"page_title"`
			SearchTerm string          `json:"search_term"`
			Ecommerce  json.RawMessage `json:"ecommerce"`
		}
		if err := json.Unmarshal(raw, &golden); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if want := strings.TrimSuffix(filepath.Base(f), ".json"); golden.Name != want {
			t.Errorf("%s names %q - a golden file is named for its event", f, golden.Name)
		}
		seen[golden.Name] = true
		r := httptest.NewRequest(http.MethodPost, TrackPath, strings.NewReader(string(raw)))
		ev, ok := browserEvent(httptest.NewRecorder(), r, uuid.New(), auth.CustomerVerifier{})
		if !ok {
			t.Errorf("%s: refused", golden.Name)
			continue
		}
		c := ev.Context
		if c.PageURL != golden.PageURL || c.Referrer != golden.Referrer || c.Locale != golden.Locale || c.PageTitle != golden.PageTitle || ev.SearchTerm != golden.SearchTerm {
			t.Errorf("%s: page or term lost: %+v %q", golden.Name, c, ev.SearchTerm)
		}
		if (golden.Ecommerce == nil) != (ev.Ecommerce == nil) {
			t.Errorf("%s: ecommerce sent %s, read %+v", golden.Name, golden.Ecommerce, ev.Ecommerce)
			continue
		}
		if ev.Ecommerce != nil {
			back, _ := json.Marshal(ev.Ecommerce)
			var sent, read any
			_ = json.Unmarshal(golden.Ecommerce, &sent)
			_ = json.Unmarshal(back, &read)
			if !reflect.DeepEqual(sent, read) {
				t.Errorf("%s: ecommerce changed on the way in\nsent %s\nread %s", golden.Name, golden.Ecommerce, back)
			}
		}
	}
	for name := range browserEvents {
		if !seen[name] {
			t.Errorf("no golden body for the browser event %s", name)
		}
	}
}

// The generated contract offers Events and marks the browser's by BrowserEvent, so a browser event missing from the
// list would never reach a storefront's types
func TestEveryBrowserEventIsAnEvent(t *testing.T) {
	for name := range browserEvents {
		if !slices.Contains(Events, name) {
			t.Errorf("%s may come from a browser but isn't in Events", name)
		}
	}
}
