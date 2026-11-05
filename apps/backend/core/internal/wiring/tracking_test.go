package wiring

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestOnlyABrowsersOwnEventsAreAccepted(t *testing.T) {
	ch := uuid.New()
	read := func(body, consent string) (tracking.Event, bool) {
		r := httptest.NewRequest(http.MethodPost, trackPath, strings.NewReader(body))
		r.RemoteAddr = "203.0.113.7:4000"
		r.Header.Set(auth.ConsentHeader, consent)
		r.Header.Set(auth.AnonHeader, "visitor-1")
		r.Header.Set(auth.ClickIDsHeader, "gclid=g1")
		return browserEvent(httptest.NewRecorder(), r, ch, auth.CustomerVerifier{})
	}
	ev, ok := read(`{"event_id":"e-1","name":"view_item","page_url":"https://shop.example/p/mug","page_title":"Blue mug","ecommerce":{"currency":"EUR","value":15,"items":[{"item_id":"SKU-1"}]}}`, "v2:a1.d0.p0")
	if !ok || ev.Name != tracking.ViewItem || ev.Source != "browser" || ev.ChannelID != ch || ev.Ecommerce.Items[0].ItemID != "SKU-1" || ev.Context.PageTitle != "Blue mug" {
		t.Fatalf("a view_item was not read: %+v", ev)
	}
	if ev.User.AnonID != "visitor-1" || ev.ClickIDs != nil || ev.Context.IP != "" {
		t.Errorf("analytics consent alone must carry the visitor and nothing for ads: %+v %+v %v", ev.User, ev.Context, ev.ClickIDs)
	}
	for name, body := range map[string]string{
		"a purchase, which only the server reports": `{"event_id":"e-2","name":"purchase"}`,
		"a cart add, which only the server reports": `{"event_id":"e-3","name":"add_to_cart"}`,
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
}
