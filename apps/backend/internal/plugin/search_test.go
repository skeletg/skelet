package plugin

import (
	"context"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/search"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func validReg(name string) sdk.SearchIndexRegistration {
	return sdk.SearchIndexRegistration{Name: name, MappingJSON: `{"mappings":{}}`}
}

func TestRegisterSearchIndexValidates(t *testing.T) {
	h := &Host{}
	if err := h.RegisterSearchIndex("acme", sdk.SearchIndexRegistration{Name: "UPPER", MappingJSON: "{}"}); err == nil {
		t.Fatal("uppercase index name accepted - it becomes part of a physical index name")
	}
	if err := h.RegisterSearchIndex("acme", sdk.SearchIndexRegistration{Name: "faq"}); err == nil {
		t.Fatal("empty mapping accepted")
	}
	if err := h.RegisterSearchIndex("acme", validReg("faq")); err != nil {
		t.Fatalf("valid registration: %v", err)
	}
	if err := h.RegisterSearchIndex("acme", validReg("faq")); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	// Same name under ANOTHER plugin is fine - the physical names diverge
	if err := h.RegisterSearchIndex("other", validReg("faq")); err != nil {
		t.Fatalf("same name on another plugin: %v", err)
	}
}

func TestSearchWriterFailsSoftWithoutTheTier(t *testing.T) {
	h := &Host{}
	if err := h.RegisterSearchIndex("acme", validReg("faq")); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Registered but the tier never arrived - a write must error, not panic or no-op
	if err := h.SearchIndexDoc(context.Background(), "acme", "faq", "1", map[string]any{"q": "hi"}); err == nil {
		t.Fatal("write succeeded with no search tier wired")
	}
	// Containment: an index the plugin never registered is refused by name, before
	// any availability check
	err := h.SearchIndexDoc(context.Background(), "acme", "not-mine", "1", nil)
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unregistered index error = %v, want 'not registered'", err)
	}
}

func TestSetSearchTierNilClientDropsPending(t *testing.T) {
	h := &Host{}
	if err := h.RegisterSearchIndex("acme", validReg("faq")); err != nil {
		t.Fatalf("register: %v", err)
	}
	h.setSearchTier(search.NewIndexRegistry(), nil)
	// The registration is remembered (so errors name the right cause) but writes are
	// permanently unavailable
	err := h.SearchIndexDoc(context.Background(), "acme", "faq", "1", nil)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("write after nil-client wire = %v, want 'not configured'", err)
	}
	// A registration arriving after the drop reports the same, rather than queueing forever
	if err := h.RegisterSearchIndex("acme", validReg("late")); err != nil {
		t.Fatalf("late register: %v", err)
	}
	if err := h.SearchIndexDoc(context.Background(), "acme", "late", "1", nil); err == nil {
		t.Fatal("late index writable with search disabled")
	}
}
