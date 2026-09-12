package signals

import (
	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/behavioralsegment"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
)

// Resolver is the Signals API's dependency root - suite services only, and permissions are deliberately not a field since they ride the request's token
type Resolver struct {
	SegmentSvc    *segment.Service
	AnalyticsSvc  *analytics.Recorder
	BehavioralSvc *behavioralsegment.Service
	ChannelSvc    *channel.Service
	ConfigSvc     *config.Service
	// The one commerce read this surface makes - which attributes are flagged for affinity tracking
	Commerce *db.Queries
	// Reads the synced roster, which is where a channel-scoped check finds a customer's channel
	Roster *signalsdb.Queries
}
