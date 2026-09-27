package boot

import (
	"go.uber.org/fx"
	"google.golang.org/grpc"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/behavioralsegment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/signals/internal/destination"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/flow"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/signals/internal/signalsboot"
	"github.com/skeletg/skelet/apps/backend/signals/internal/signalsgrpc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/suppression"
)

// Contract is the wire surface core calls, and the upkeep it schedules - cmd/signals alone serves it,
// so the beacon tier never builds a handler it would not answer
var Contract = fx.Module("contract", fx.Provide(newGRPCServer))

type contractDeps struct {
	fx.In

	Config        Config
	Queries       Queries
	ChannelScoped *config.ChannelScoped
	Revocations   auth.Revocations
	Stack         *signalsboot.Stack
	Recorder      *analytics.Recorder
	Roster        *analytics.Roster
	Traits        *analytics.TraitCompute
	Channels      *analytics.ChannelRoster
	Lens          *lens.Service
	Experiment    *experiment.Service
	Segment       *segment.Service
	Behavioral    *behavioralsegment.Service
	Destination   *destination.Service
	SendLog       *sendlog.Store
	Suppression   *suppression.Service
	Flow          *flow.Service
	Campaign      *campaign.Service
	Builder       *emailtemplate.Service
	Stores        *RosterStores
	Contacts      *RosterContacts
}

func newGRPCServer(d contractDeps) *grpc.Server {
	return signalsgrpc.NewServer(signalsgrpc.Deps{
		Experiment:     d.Experiment,
		Lens:           d.Lens,
		Destination:    d.Destination,
		Segment:        d.Segment,
		Analytics:      d.Recorder,
		CustomerRoster: d.Roster,
		Traits:         d.Traits,
		Behavioral:     d.Behavioral,
		Roster:         d.Queries.Queries,
		Channels:       d.Channels,
		Settings:       d.ChannelScoped,
		Queue:          d.Stack.Queue,
		SettingsRepo:   signalsboot.NewSettingsRepository(d.Queries.Queries),
		SendLog:        d.SendLog,
		Suppression:    d.Suppression,
		Flows:          d.Flow,
		Campaigns:      d.Campaign,
		Builder:        d.Builder,
		Stores:         d.Stores,
		Contacts:       PreviewContacts{RosterContacts: d.Contacts},
		ServiceSecret:  d.Config.ServiceSecret,
		Revocations:    d.Revocations,
		Dev:            d.Config.IsDev(),
	})
}
