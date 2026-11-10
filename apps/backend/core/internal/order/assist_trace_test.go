package order

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
)

func TestOnlyTheAssistantsCreditHandsItsTurnToTheOrder(t *testing.T) {
	c := cart.Cart{AssistTrace: "4bf92f3577b34da6a3ce929d0e0e4736"}
	if got := assistTrace(cart.AssistedAssistant, c); got != c.AssistTrace {
		t.Errorf("assistant credit = %q", got)
	}
	for _, credit := range []string{cart.AssistedAgent, ""} {
		if got := assistTrace(credit, c); got != "" {
			t.Errorf("credit %q handed on %q - an agent's or a stale credit has no turn to score", credit, got)
		}
	}
}
