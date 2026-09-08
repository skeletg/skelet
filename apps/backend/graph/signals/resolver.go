package signals

import (
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
)

// Resolver is the Signals API's dependency root - suite services only, and permissions are deliberately not a field since they ride the request's token
type Resolver struct {
	ExperimentSvc *experiment.Service
}
