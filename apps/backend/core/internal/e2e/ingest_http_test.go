package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/form"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// The commerce beacons - signup forms and "notify me when it's back" - are core's own routes under /t,
// served by the root handler ahead of the proxy to the suite

// tRequest drives one /t call through the real root handler - channel middleware, rate limiter and all - extra headers come in pairs
func tRequest(t *testing.T, ts *httptest.Server, method, path, channelID, body string, hdr ...string) (int, http.Header, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL+path, rdr)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if channelID != "" {
		req.Header.Set("X-Channel-Id", channelID)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, resp.Header, string(raw)
}

func TestIngestBackInStockRecordsTheRequest(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 0, 1000)
	ts := httptest.NewServer(h.D.Root)
	defer ts.Close()

	pending := func() []db.ListPendingBackInStockRow {
		rows, err := h.D.Queries.ListPendingBackInStock(h.Ctx, db.ListPendingBackInStockParams{
			ProductID: f.Product.ID, ChannelID: f.Channel.ID,
		})
		if err != nil {
			t.Fatalf("list pending: %v", err)
		}
		return rows
	}

	// a guest subscribes from the PDP - the email is normalized, no customer attached
	body := fmt.Sprintf(`{"productId":%q,"sku":%q,"email":"  Guest@E2E.test ","name":"Tire","url":"https://shop/p"}`, f.Product.ID, f.Product.SKU)
	status, _, _ := tRequest(t, ts, http.MethodPost, "/t/back-in-stock", f.Channel.ID.String(), body)
	if status != http.StatusNoContent {
		t.Fatalf("POST /t/back-in-stock = %d, want 204", status)
	}
	rows := pending()
	if len(rows) != 1 {
		t.Fatalf("%d pending requests, want 1", len(rows))
	}
	if rows[0].Email != "guest@e2e.test" {
		t.Errorf("email = %q, want it lowered and trimmed", rows[0].Email)
	}
	if rows[0].CustomerID.Valid {
		t.Error("a guest request carries a customer id")
	}

	// a logged-in shopper's identity comes from the session cookie, never the body
	cust, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, "bis-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	tok := h.customerToken(t, cust.ID, f.Channel.ID, cust.Email)
	body = fmt.Sprintf(`{"productId":%q,"email":%q}`, f.Product.ID, cust.Email)
	status, _, _ = tRequest(t, ts, http.MethodPost, "/t/back-in-stock", f.Channel.ID.String(), body, "Cookie", "storefront_access_token="+tok)
	if status != http.StatusNoContent {
		t.Fatalf("authenticated POST = %d, want 204", status)
	}
	rows = pending()
	if len(rows) != 2 {
		t.Fatalf("%d pending requests, want 2", len(rows))
	}
	var authed *db.ListPendingBackInStockRow
	for i := range rows {
		if rows[i].Email == cust.Email {
			authed = &rows[i]
		}
	}
	if authed == nil {
		t.Fatal("the authenticated request is not in the waiting list")
	}
	if !authed.CustomerID.Valid || authed.CustomerID.UUID != cust.ID {
		t.Errorf("customer = %+v, want the session cookie's customer resolved", authed.CustomerID)
	}

	// junk stays out but still answers 204, so a prober learns nothing
	for name, junk := range map[string]string{
		"no email":       fmt.Sprintf(`{"productId":%q}`, f.Product.ID),
		"bad product id": `{"productId":"nope","email":"a@b.test"}`,
		"not json":       `<xml/>`,
	} {
		status, _, _ := tRequest(t, ts, http.MethodPost, "/t/back-in-stock", f.Channel.ID.String(), junk)
		if status != http.StatusNoContent {
			t.Errorf("%s: status = %d, want the silent 204", name, status)
		}
	}
	if got := pending(); len(got) != 2 {
		t.Errorf("%d pending after the junk posts, want still 2", len(got))
	}
}

func TestIngestFormsServeAndSubmit(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 1000)
	ts := httptest.NewServer(h.D.Root)
	defer ts.Close()

	created, err := h.D.Form.Create(h.Ctx, form.SaveInput{
		ChannelID: f.Channel.ID, Slug: "e2e-news-" + uuid.NewString()[:8], Name: "Newsletter",
		Status: "active", Type: "popup", Heading: "Join us", ButtonLabel: "Sign up",
		SuccessMessage: "Welcome aboard!", CollectName: true,
		Display: form.DisplayConfig{Trigger: "immediate"},
	})
	if err != nil {
		t.Fatalf("create form: %v", err)
	}

	status, _, body := tRequest(t, ts, http.MethodGet, "/t/forms", f.Channel.ID.String(), "")
	if status != http.StatusOK {
		t.Fatalf("GET /t/forms = %d %q", status, body)
	}
	var forms []map[string]any
	if err := json.Unmarshal([]byte(body), &forms); err != nil {
		t.Fatalf("forms response: %v\n%s", err, body)
	}
	var mine map[string]any
	for _, fm := range forms {
		if fm["slug"] == created.Slug {
			mine = fm
		}
	}
	if mine == nil {
		t.Fatalf("the active form is not served: %s", body)
	}
	if mine["heading"] != "Join us" || mine["successMessage"] != "Welcome aboard!" {
		t.Errorf("form projection = %v, want the storefront fields", mine)
	}
	// the projection is public-safe - segment and consent internals stay server-side
	for key := range mine {
		if strings.Contains(strings.ToLower(key), "segment") || strings.Contains(strings.ToLower(key), "consent") {
			t.Errorf("public form payload leaks %q", key)
		}
	}

	status, _, body = tRequest(t, ts, http.MethodPost, "/t/forms/submit",
		f.Channel.ID.String(), fmt.Sprintf(`{"form":%q,"email":"lead@e2e.test","name":"Ana"}`, created.Slug))
	if status != http.StatusOK {
		t.Fatalf("submit = %d %q", status, body)
	}
	var out map[string]string
	_ = json.Unmarshal([]byte(body), &out)
	if out["message"] != "Welcome aboard!" {
		t.Errorf("message = %q, want the form's success message", out["message"])
	}
	n, err := h.D.Form.SubmissionCount(h.Ctx, created.ID)
	if err != nil || n != 1 {
		t.Errorf("submissions = %d (%v), want the lead recorded once", n, err)
	}

	// a broken body is the caller's fault and says so - unlike the beacons, the widget shows this error
	status, _, _ = tRequest(t, ts, http.MethodPost, "/t/forms/submit", f.Channel.ID.String(), `{"form":`)
	if status != http.StatusBadRequest {
		t.Errorf("malformed submit = %d, want 400", status)
	}
	status, _, body = tRequest(t, ts, http.MethodPost, "/t/forms/submit", f.Channel.ID.String(), `{"form":"no-such-form","email":"a@b.test"}`)
	if status != http.StatusBadRequest {
		t.Errorf("unknown form = %d %q, want 400", status, body)
	}

	// the master switch takes every form off the storefront without touching them
	if _, err := h.D.Config.Set(h.Ctx, "outreach/forms/enabled", "false", config.DefaultScope); err != nil {
		t.Fatalf("flip master switch: %v", err)
	}
	defer func() { _, _ = h.D.Config.Set(h.Ctx, "outreach/forms/enabled", "true", config.DefaultScope) }()
	_, _, body = tRequest(t, ts, http.MethodGet, "/t/forms", f.Channel.ID.String(), "")
	if strings.TrimSpace(body) != "[]" {
		t.Errorf("forms with the switch off = %q, want an empty list", body)
	}
}
