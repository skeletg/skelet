package webhook

import (
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/circuit"
)

// Circuit breaker for sync webhook targets, a thin wrapper over the shared internal/circuit breaker - a subscriber that keeps failing stops being called for a cooldown window instead of adding its timeout to every user-facing operation it gates

type CircuitState string

const (
	CircuitClosed   CircuitState = CircuitState(circuit.Closed)
	CircuitOpen     CircuitState = CircuitState(circuit.Open)
	CircuitHalfOpen CircuitState = CircuitState(circuit.HalfOpen)
)

const (
	breakerTripThreshold = 5
	// How long an open circuit swallows calls before allowing one probe
	breakerCooldown = 30 * time.Second
)

type CircuitInfo struct {
	State               CircuitState
	ConsecutiveFailures int
	OpenedAt            *time.Time
}

var circuits = circuit.New(breakerTripThreshold, breakerCooldown)

// Exposes breaker state to the admin API - only meaningful for sync webhooks, since async attempts never feed it
func CircuitInfoFor(id uuid.UUID) CircuitInfo {
	info := circuits.Info(id)
	return CircuitInfo{
		State:               CircuitState(info.State),
		ConsecutiveFailures: info.ConsecutiveFailures,
		OpenedAt:            info.OpenedAt,
	}
}

func ResetCircuit(id uuid.UUID) {
	circuits.Reset(id)
}
