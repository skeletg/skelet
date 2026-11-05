package wiring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type countedViews []signalsclient.ActivitySpec

func (c *countedViews) RecordActivity(_ context.Context, a signalsclient.ActivitySpec) error {
	*c = append(*c, a)
	return nil
}

type aggregateSetting bool

func (s aggregateSetting) GetBool(context.Context, string, config.Scope) (bool, error) {
	return bool(s), nil
}

// A shop that turned aggregate counts off records nothing for a visitor who declined analytics - the privacy setting's promise
func TestACountedViewHonoursTheShopsSetting(t *testing.T) {
	ch := &channel.Channel{ID: uuid.New()}
	post := func(on bool, method string) countedViews {
		var got countedViews
		r := httptest.NewRequest(method, countPath, strings.NewReader(`{"path":"/en/products/blue-mug"}`))
		r = r.WithContext(channel.WithChannel(r.Context(), ch))
		w := httptest.NewRecorder()
		countView(&got, aggregateSetting(on), nil)(w, r)
		if w.Code != http.StatusNoContent {
			t.Errorf("status %d, want 204 whatever happens", w.Code)
		}
		return got
	}
	if got := post(true, http.MethodPost); len(got) != 1 || got[0].Source != "aggregate" || got[0].ChannelID != ch.ID || got[0].Props["path"] != "/en/products/blue-mug" {
		t.Errorf("setting on: recorded %+v, want the one view as nobody", got)
	}
	if got := post(false, http.MethodPost); len(got) != 0 {
		t.Errorf("setting off: recorded %+v, want nothing", got)
	}
	if got := post(true, http.MethodGet); len(got) != 0 {
		t.Errorf("a GET recorded %+v, want nothing", got)
	}
}

func TestACountedPathNamesNoOrderAndNoQuery(t *testing.T) {
	for body, want := range map[string]string{
		`{"path":"/en/products/blue-mug"}`:                                    "/en/products/blue-mug",
		`{"path":"/en/products/organic-cotton-crew-neck-tee-2026"}`:           "/en/products/organic-cotton-crew-neck-tee-2026",
		`{"path":"/orders/0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b?email=a@b.c"}`: "/orders/:id",
		`{"path":"/account/orders/1234567"}`:                                  "/account/orders/:id",
		`{"path":"/reset/abcdefghijklmnopqrstuvwxyz012345#x"}`:                "/reset/:id",
	} {
		r := httptest.NewRequest(http.MethodPost, countPath, strings.NewReader(body))
		if got, ok := countedPath(httptest.NewRecorder(), r); !ok || got != want {
			t.Errorf("%s: %q, %v - want %q", body, got, ok, want)
		}
	}
	for _, body := range []string{`{"path":"no-slash"}`, `nope`, `{"path":"/` + strings.Repeat("a", 600) + `"}`} {
		if _, ok := countedPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, countPath, strings.NewReader(body))); ok {
			t.Errorf("accepted %s", body)
		}
	}
}
