package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

func testClient(ts *httptest.Server) *Client {
	return &Client{BaseURL: ts.URL, Name: "gw", IdempotencyHeader: "Idempotency-Key",
		Authorize: func(r *http.Request) { r.Header.Set("Authorization", "Bearer t") }}
}

func TestDoJSONRoundTrip(t *testing.T) {
	var gotAuth, gotIdem, gotCT string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotIdem, gotCT = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key"), r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{"id":"tx_1"}`))
	}))
	defer ts.Close()

	var out struct {
		ID string `json:"id"`
	}
	if _, err := testClient(ts).DoJSON(context.Background(), http.MethodPost, "/v1/tx", map[string]any{"a": 1}, &out, "key-1"); err != nil {
		t.Fatal(err)
	}
	if out.ID != "tx_1" || gotAuth != "Bearer t" || gotIdem != "key-1" || gotCT != "application/json" {
		t.Errorf("got id=%q auth=%q idem=%q ct=%q", out.ID, gotAuth, gotIdem, gotCT)
	}
}

// The defect this package exists for: a torn body on a 200 must surface as an error, never as a
// zero-valued result the caller records as a successful capture
func TestTornBodyIsAnError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000") // promise more than is sent, then cut the connection
		_, _ = w.Write([]byte(`{"id":"tx`))
	}))
	defer ts.Close()

	var out struct{ ID string }
	_, err := testClient(ts).DoJSON(context.Background(), http.MethodPost, "/v1/tx", nil, &out, "")
	if err == nil {
		t.Fatalf("torn body returned nil error with out=%+v", out)
	}
	if !strings.Contains(err.Error(), "read response") {
		t.Errorf("error should name the read: %v", err)
	}
}

func TestNonTwoxxIsHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"no such order"}`, http.StatusNotFound)
	}))
	defer ts.Close()

	raw, err := testClient(ts).DoJSON(context.Background(), http.MethodGet, "/v1/tx/x", nil, nil, "")
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 404 {
		t.Fatalf("want *HTTPError 404, got %v", err)
	}
	if !strings.Contains(string(raw), "no such order") {
		t.Errorf("body should ride back for envelope decoding, got %q", raw)
	}
	if !IsStatus(err, 404) || IsStatus(err, 500) {
		t.Error("IsStatus misclassified")
	}
}

func TestNoIdempotencyHeaderConfiguredDropsKey(t *testing.T) {
	var idem string
	var present bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idem = r.Header.Get("Idempotency-Key")
		_, present = r.Header["Idempotency-Key"]
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	c := &Client{BaseURL: ts.URL, Name: "gw"} // gateway with no dedupe header
	if _, err := c.DoJSON(context.Background(), http.MethodPost, "/x", nil, nil, "key-1"); err != nil {
		t.Fatal(err)
	}
	if present || idem != "" {
		t.Errorf("key must be dropped when the gateway has no header, got %q", idem)
	}
}

func TestDoForm(t *testing.T) {
	var gotBody, gotCT string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody, gotCT = string(b), r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	form := url.Values{}
	form.Set("amount", "100")
	if _, err := testClient(ts).DoForm(context.Background(), http.MethodPost, "/x", form, nil, ""); err != nil {
		t.Fatal(err)
	}
	if gotBody != "amount=100" || gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("got body=%q ct=%q", gotBody, gotCT)
	}
}

func TestBodyCap(t *testing.T) {
	size := 100
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", size)))
	}))
	defer ts.Close()

	c := &Client{BaseURL: ts.URL, Name: "gw", MaxBodyBytes: 10}
	// a cut-off 2xx body is a torn response, not a shorter success
	if raw, err := c.DoJSON(context.Background(), http.MethodGet, "/x", nil, nil, ""); err == nil || !strings.Contains(err.Error(), "cap") {
		t.Errorf("over-cap body answered %d bytes, err %v - want an error naming the cap", len(raw), err)
	}
	size = 10
	if raw, err := c.DoJSON(context.Background(), http.MethodGet, "/x", nil, nil, ""); err != nil || len(raw) != 10 {
		t.Errorf("body exactly at the cap: %d bytes, err %v - want it whole", len(raw), err)
	}
}

func TestTruncateKeepsUTF8Whole(t *testing.T) {
	s := Truncate([]byte(strings.Repeat("a", 399) + "é" + strings.Repeat("b", 10)))
	if !utf8.ValidString(s) {
		t.Errorf("Truncate split a rune: %q", s[len(s)-8:])
	}
}
