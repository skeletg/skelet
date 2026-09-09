package signals

import (
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/destination"
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
)

// Resolver is the Signals API's dependency root - suite services only, and permissions are deliberately not a field since they ride the request's token
type Resolver struct {
	ExperimentSvc  *experiment.Service
	LensSvc        *lens.Service
	DestinationSvc *destination.Service
	// Reads the synced roster, which is where a channel-scoped check finds a customer's channel
	Roster *signalsdb.Queries
}
