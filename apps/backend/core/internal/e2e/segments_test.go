package e2e

import (
	"testing"

	"github.com/google/uuid"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// newSegment creates an audience the way the dashboard does - over the contract, as an admin who may
// manage promotions - since segments are the suite's and core holds no service for them any more
func (h *harness) newSegment(t *testing.T, slug string) uuid.UUID {
	t.Helper()
	adm, err := h.D.Admin.CreateAdmin(h.Ctx, "seg-"+uuid.NewString()[:8]+"@e2e.test", "segment-e2e-password-1", "Segment", "Admin", []string{"administrator"})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	actx, err := h.D.SignalsClient.AsAdmin(h.Ctx, adm.ID)
	if err != nil {
		t.Fatalf("admin token: %v", err)
	}
	res, err := h.D.SignalsClient.Segments.CreateSegment(actx, &signalsv1.CreateSegmentRequest{Slug: slug, Name: "E2E " + slug})
	if err != nil {
		t.Fatalf("create segment over the contract: %v", err)
	}
	id, err := uuid.Parse(res.GetSegment().GetId())
	if err != nil {
		t.Fatalf("segment id %q: %v", res.GetSegment().GetId(), err)
	}
	return id
}
