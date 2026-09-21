package admin

import (
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/health"
)

// Lives outside the .resolvers.go file because gqlgen relocates every non-resolver
// declaration it finds there
func toHealthStatus(s health.Status) model.HealthStatus {
	switch s {
	case health.StatusOK:
		return model.HealthStatusOk
	case health.StatusWarn:
		return model.HealthStatusWarn
	case health.StatusBad:
		return model.HealthStatusBad
	default:
		return model.HealthStatusOff
	}
}
