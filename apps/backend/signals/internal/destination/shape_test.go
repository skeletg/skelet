package destination

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func evt(name, source, anon string, custID *uuid.UUID, props string) Event {
	return Event{
		ID: uuid.New(), Name: name, Source: source, AnonID: anon,
		CustomerID: custID, OccurredAt: time.Unix(1700000000, 0),
		Properties: json.RawMessage(props),
	}
}

func TestHashIdentifier(t *testing.T) {
	// Meta/Google recipe: trim + lowercase before SHA-256
	want := "unique"
	a := HashIdentifier("  Alice@Example.COM ")
	b := HashIdentifier("alice@example.com")
	if a != b {
		t.Errorf("normalization mismatch: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Errorf("want 64 hex chars, got %d (%s)", len(a), want)
	}
}

func TestShapeWebhook(t *testing.T) {
	body, err := ShapePayload(KindWebhook, []Event{evt("page_view", "web", "anon-1", nil, `{"path":"/"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Count  int `json:"count"`
		Events []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 1 || out.Events[0].Name != "page_view" {
		t.Errorf("webhook shape wrong: %s", body)
	}
}

func TestShapeGA4GroupsByClient(t *testing.T) {
	cust := uuid.New()
	body, err := ShapePayload(KindGA4, []Event{
		evt("Page View", "web", "anon-1", nil, `{"path":"/a"}`),
		evt("page_view", "web", "anon-1", nil, `{"path":"/b"}`),
		evt("email_opened", "campaign", "", &cust, `{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var envs []struct {
		ClientID string `json:"client_id"`
		Events   []struct {
			Name string `json:"name"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &envs); err != nil {
		t.Fatal(err)
	}
	if len(envs) != 2 {
		t.Fatalf("want 2 MP envelopes (2 identities), got %d", len(envs))
	}
	if envs[0].ClientID != "anon-1" || len(envs[0].Events) != 2 {
		t.Errorf("anon envelope wrong: %+v", envs[0])
	}
	if envs[0].Events[0].Name != "page_view" {
		t.Errorf("GA4 names must snake_case: %q", envs[0].Events[0].Name)
	}
	if envs[1].ClientID != cust.String() {
		t.Errorf("customer fallback client_id wrong: %+v", envs[1])
	}
}

func TestShapeMetaCAPI(t *testing.T) {
	cust := uuid.New()
	body, err := ShapePayload(KindMetaCAPI, []Event{
		evt("conversion", "campaign", "", &cust, `{"value":42}`),
		evt("page_view", "web", "anon-9", nil, `{}`),
		evt("cart_add", "commerce", "anon-9", nil, `{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Data []struct {
			EventName    string         `json:"event_name"`
			EventTime    int64          `json:"event_time"`
			EventID      string         `json:"event_id"`
			ActionSource string         `json:"action_source"`
			UserData     map[string]any `json:"user_data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 3 {
		t.Fatalf("want 3 CAPI events, got %d", len(out.Data))
	}
	// campaign -> email, web -> website, commerce -> system_generated
	if out.Data[0].ActionSource != "email" || out.Data[1].ActionSource != "website" || out.Data[2].ActionSource != "system_generated" {
		t.Errorf("action_source mapping wrong: %+v", out.Data)
	}
	if out.Data[0].EventTime != 1700000000 || out.Data[0].EventID == "" {
		t.Errorf("time/dedupe id missing: %+v", out.Data[0])
	}
	ext, _ := out.Data[0].UserData["external_id"].([]any)
	if len(ext) != 1 || ext[0] != HashIdentifier(cust.String()) {
		t.Errorf("external_id must be hashed customer id: %+v", out.Data[0].UserData)
	}
}

// The identifiers arrive hashed, so this covers the envelope and the blank-dropping, not the recipe
func TestBuildAudiencePayload(t *testing.T) {
	in := []string{HashIdentifier("alice@example.com"), HashIdentifier("bob@example.com"), ""}
	body, hashes := BuildAudiencePayload("VIP", in)
	if len(hashes) != 2 {
		t.Fatalf("blank identifiers must be dropped, got %d", len(hashes))
	}
	if hashes[0] != in[0] {
		t.Error("an identifier must go out as it came in - this service cannot re-hash what it never saw")
	}
	var out struct {
		Schema      string   `json:"schema"`
		Identifiers []string `json:"identifiers"`
		MemberCount int      `json:"member_count"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Schema != "EMAIL_SHA256" || out.MemberCount != 2 {
		t.Errorf("payload envelope wrong: %s", body)
	}
}

func TestMatchesFilters(t *testing.T) {
	e := evt("page_view", "web", "a", nil, `{}`)
	if !matchesFilters(e, nil, nil) {
		t.Error("empty filters must match everything")
	}
	if !matchesFilters(e, []string{"WEB"}, []string{"Page_View"}) {
		t.Error("filters must be case-insensitive")
	}
	if matchesFilters(e, []string{"campaign"}, nil) {
		t.Error("source filter must exclude")
	}
	if matchesFilters(e, nil, []string{"conversion"}) {
		t.Error("name filter must exclude")
	}
}
