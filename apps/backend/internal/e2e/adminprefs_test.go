package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skeletg/skelet/internal/integration"
)

// adminGqlAuthed drives the admin API as a signed-in admin - Bearer JWT through the whole
// production chain, variables included since the prefs mutation carries a JSON value
func (h *harness) adminGqlAuthed(t *testing.T, ts *httptest.Server, bearer, query string, variables map[string]any) (map[string]any, []string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/graphql", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("post admin graphql: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode admin graphql response (http %d): %v", resp.StatusCode, err)
	}
	var msgs []string
	for _, e := range out.Errors {
		msgs = append(msgs, e.Message)
	}
	return out.Data, msgs
}

// Scenario: per-admin preferences over the real API - a signed-in admin writes and reads back their own prefs, a null value deletes, and
// the two non-admin callers (anonymous, integration token) are refused - prefs are person-shaped state, machine tokens have no admin row
func TestAdminPreferencesRoundtrip(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.d.Root.Handler)
	defer ts.Close()

	email := fmt.Sprintf("prefs-e2e-%d@example.com", time.Now().UnixNano())
	if _, err := h.d.Admin.CreateAdmin(h.ctx, email, "prefs-e2e-password-1", "Prefs", "Tester", nil); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	login, errs := h.adminGqlAuthed(t, ts, "", `mutation($e: String!, $p: String!) {
		adminLogin(input: {email: $e, password: $p}) { accessToken }
	}`, map[string]any{"e": email, "p": "prefs-e2e-password-1"})
	if len(errs) > 0 {
		t.Fatalf("adminLogin: %v", errs)
	}
	bearer, _ := login["adminLogin"].(map[string]any)["accessToken"].(string)
	if bearer == "" {
		t.Fatal("adminLogin returned no access token")
	}

	const setMutation = `mutation($k: String!, $v: JSON) { setMyPreference(key: $k, value: $v) }`

	if _, errs := h.adminGqlAuthed(t, ts, bearer, setMutation, map[string]any{
		"k": "nav:pins", "v": []string{"/orders", "/webhooks"},
	}); len(errs) > 0 {
		t.Fatalf("setMyPreference: %v", errs)
	}

	data, errs := h.adminGqlAuthed(t, ts, bearer, `query { myPreferences { key value } }`, nil)
	if len(errs) > 0 {
		t.Fatalf("myPreferences: %v", errs)
	}
	prefs, _ := data["myPreferences"].([]any)
	if len(prefs) != 1 {
		t.Fatalf("myPreferences = %v, want exactly the one key", data)
	}
	row := prefs[0].(map[string]any)
	if row["key"] != "nav:pins" {
		t.Errorf("key = %v, want nav:pins", row["key"])
	}
	if pins, _ := row["value"].([]any); len(pins) != 2 || pins[0] != "/orders" {
		t.Errorf("value = %v, want the two pins back", row["value"])
	}

	// null value deletes the key
	if _, errs := h.adminGqlAuthed(t, ts, bearer, setMutation, map[string]any{"k": "nav:pins", "v": nil}); len(errs) > 0 {
		t.Fatalf("setMyPreference nil: %v", errs)
	}
	data, _ = h.adminGqlAuthed(t, ts, bearer, `query { myPreferences { key } }`, nil)
	if prefs, _ := data["myPreferences"].([]any); len(prefs) != 0 {
		t.Errorf("after nil-set myPreferences = %v, want empty", prefs)
	}

	// anonymous callers are refused
	if _, errs := h.adminGqlAuthed(t, ts, "", `query { myPreferences { key } }`, nil); !errorsContain(errs, "unauthenticated") {
		t.Errorf("anonymous myPreferences = %v, want unauthenticated", errs)
	}

	// integration tokens authorize resolvers by grant set, but prefs demand an admin identity
	tok, err := h.d.Integration.Create(h.ctx, integration.CreateIntegrationInput{Name: "prefs e2e token"})
	if err != nil {
		t.Fatalf("create integration: %v", err)
	}
	defer func() { _ = h.d.Integration.Delete(h.ctx, tok.Integration.ID) }()
	if _, errs := h.adminGqlPost(t, ts, tok.RawToken, `query { myPreferences { key } }`); !errorsContain(errs, "unauthenticated") {
		t.Errorf("integration-token myPreferences = %v, want unauthenticated", errs)
	}
}
