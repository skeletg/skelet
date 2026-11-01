package wiring

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
