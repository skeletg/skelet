package flow

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type staticVars map[string]string

func (v staticVars) MergeVars(context.Context, uuid.UUID, uuid.UUID) map[string]string { return v }

// The engine's own tags are authoritative - a provider must not be able to redefine the address a mail goes to
func TestProvidersCannotOverwriteEngineTags(t *testing.T) {
	s := &Service{}
	s.RegisterMergeVars(staticVars{"customer.email": "attacker@example.com", "customer.lifecycle": "active"})

	m := map[string]string{"customer.email": "real@example.com"}
	s.varSources.merge(context.Background(), uuid.New(), uuid.New(), m)

	if m["customer.email"] != "real@example.com" {
		t.Errorf("customer.email = %q, want the engine's own value kept", m["customer.email"])
	}
	if m["customer.lifecycle"] != "active" {
		t.Error("a provider's own tag did not land")
	}
}

// No suite registered means the tags are simply absent, which is what a failed read already produced
func TestNoProvidersLeavesTagsAbsent(t *testing.T) {
	s := &Service{}
	m := map[string]string{"store.name": "Shop"}
	s.varSources.merge(context.Background(), uuid.New(), uuid.New(), m)
	if len(m) != 1 {
		t.Errorf("merge added %d tags with no providers, want none", len(m)-1)
	}
}
