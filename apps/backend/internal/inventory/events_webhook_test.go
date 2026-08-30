package inventory

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestAllocatingApplyWebhookResponse(t *testing.T) {
	lineA, lineB := uuid.New(), uuid.New()
	locX, locY := uuid.New(), uuid.New()
	var routing map[uuid.UUID]uuid.UUID
	e := &Allocating{
		Lines:   []ReserveLine{{OrderLineID: lineA}, {OrderLineID: lineB}},
		Routing: &routing,
	}

	body := fmt.Appendf(nil, `{"routing": {"%s": "%s"}}`, lineA, locX)
	if reject, _, err := e.ApplyWebhookResponse(body); err != nil || reject {
		t.Fatalf("apply: reject=%v err=%v", reject, err)
	}
	if routing[lineA] != locX {
		t.Fatalf("routing not applied: %v", routing)
	}

	// second webhook merges + overwrites per line
	body = fmt.Appendf(nil, `{"routing": {"%s": "%s", "%s": "%s"}}`, lineA, locY, lineB, locY)
	if _, _, err := e.ApplyWebhookResponse(body); err != nil {
		t.Fatal(err)
	}
	if routing[lineA] != locY || routing[lineB] != locY {
		t.Fatalf("merge semantics wrong: %v", routing)
	}

	// unknown line id is a hard error
	body = fmt.Appendf(nil, `{"routing": {"%s": "%s"}}`, uuid.New(), locX)
	if _, _, err := e.ApplyWebhookResponse(body); err == nil {
		t.Fatal("unknown line id should error")
	}

	// veto still works
	if reject, reason, _ := e.ApplyWebhookResponse([]byte(`{"reject": true, "reason": "dc offline"}`)); !reject || reason != "dc offline" {
		t.Fatal("veto not honored")
	}
}
