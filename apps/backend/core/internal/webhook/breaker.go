package webhook

import (
	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/circuit"
)

// Circuit breaker for sync webhook targets, a thin wrapper over the shared internal/circuit breaker - a subscriber that keeps failing stops being called for a cooldown window instead of adding its timeout to every user-facing operation it gates

var circuits = circuit.New(circuit.DefaultThreshold, circuit.DefaultCooldown)

// Exposes breaker state to the admin API - only meaningful for sync webhooks, since async attempts never feed it
func CircuitInfoFor(id uuid.UUID) circuit.Info {
	return circuits.Info(id)
}

func ResetCircuit(id uuid.UUID) {
	circuits.Reset(id)
}
