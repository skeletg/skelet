package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skeletg/skelet/internal/integration"
)

// adminGqlPost drives the real admin API with an integration token - the request crosses the
// whole production chain: recover, body limits, JWT middleware, token middleware, session guard
func (h *harness) adminGqlPost(t *testing.T, ts *httptest.Server, token, query string) (map[string]any, []string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/graphql", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set(integration.HeaderAPIToken, token)
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

func errorsContain(msgs []string, want string) bool {
	for _, m := range msgs {
		if strings.Contains(strings.ToLower(m), want) {
			return true
		}
	}
	return false
}

// Scenario: a machine integration calls the admin API - the token IS the whole
// authorization story: a granted slug answers through the same resolver gate an admin role uses,
// an ungranted slug is refused, no token reads as anonymous, and disabling the integration kills
// access on the very next request
func TestAdminGraphQLIntegrationToken(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.d.Root.Handler)
	defer ts.Close()

	granted, err := h.d.Integration.Create(h.ctx, integration.CreateIntegrationInput{
		Name: "e2e ERP bridge", Permissions: []string{"admins.manage"},
	})
	if err != nil {
		t.Fatalf("create granted integration: %v", err)
	}
	defer func() { _ = h.d.Integration.Delete(h.ctx, granted.Integration.ID) }()
	ungranted, err := h.d.Integration.Create(h.ctx, integration.CreateIntegrationInput{
		Name: "e2e read-only feed", Permissions: []string{"orders.view"},
	})
	if err != nil {
		t.Fatalf("create ungranted integration: %v", err)
	}
	defer func() { _ = h.d.Integration.Delete(h.ctx, ungranted.Integration.ID) }()

	const query = `query { integrations { id name } }` // gated by admins.manage - channel-free, so the test needs no store fixture

	if _, errs := h.adminGqlPost(t, ts, "", query); !errorsContain(errs, "unauthenticated") {
		t.Errorf("no token = %v, want unauthenticated", errs)
	}

	if _, errs := h.adminGqlPost(t, ts, ungranted.RawToken, query); !errorsContain(errs, "forbidden") {
		t.Errorf("token without the slug = %v, want forbidden", errs)
	}

	data, errs := h.adminGqlPost(t, ts, granted.RawToken, query)
	if len(errs) > 0 {
		t.Fatalf("granted token: %v", errs)
	}
	rows, ok := data["integrations"].([]any)
	if !ok {
		t.Fatalf("granted token returned no integrations list: %v", data)
	}
	var sawSelf bool
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok && m["name"] == "e2e ERP bridge" {
			sawSelf = true
		}
	}
	if !sawSelf {
		t.Error("the listing is missing the integration that made the call - the grant answered with someone else's data")
	}

	if _, err := h.d.Integration.SetActive(h.ctx, granted.Integration.ID, false); err != nil {
		t.Fatalf("disable integration: %v", err)
	}
	if _, errs := h.adminGqlPost(t, ts, granted.RawToken, query); !errorsContain(errs, "unauthenticated") {
		t.Errorf("disabled integration = %v, want unauthenticated on the next request", errs)
	}
}
