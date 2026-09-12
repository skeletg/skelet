package wiring

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/vektah/gqlparser/v2/ast"

	graphsignals "github.com/skeletg/skelet/apps/backend/graph/signals"
	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/behavioralsegment"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/graphqlx"
	"github.com/skeletg/skelet/apps/backend/internal/observability"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/signalsapi"
)

// SignalsAPI is what the Signals GraphQL surface needs - suite services only, since this process has no commerce services to offer
type SignalsAPI struct {
	Segment    *segment.Service
	Analytics  *analytics.Recorder
	Behavioral *behavioralsegment.Service
	Channel    *channel.Service
	Config     *config.Service
	Roster     *signalsdb.Queries
	Commerce   *db.Queries
	JWTSecret  string
	Dev        bool
	Telemetry  observability.Telemetry
	// Optional - nil means a session killed in core keeps working here until its token expires
	Revocations auth.Revocations
}

// SignalsGraphQLHandler builds the Signals API handler, already wrapped in its own authentication
func SignalsGraphQLHandler(a SignalsAPI) http.Handler {
	srv := handler.New(graphsignals.NewExecutableSchema(graphsignals.Config{
		Resolvers: &graphsignals.Resolver{
			SegmentSvc:   a.Segment,
			AnalyticsSvc: a.Analytics, BehavioralSvc: a.Behavioral, ChannelSvc: a.Channel, ConfigSvc: a.Config, Commerce: a.Commerce, Roster: a.Roster,
		},
	}))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.SetQueryCache(lru.New[*ast.QueryDocument](200))
	if a.Dev {
		srv.Use(extension.Introspection{})
	}
	srv.Use(extension.FixedComplexityLimit(500))

	opts := []graphqlx.Option{graphqlx.WithReporter(a.Telemetry.Reporter)}
	srv.SetRecoverFunc(graphqlx.RecoverFunc(graphqlx.SurfaceSignals, opts...))
	srv.SetErrorPresenter(graphqlx.Presenter(graphqlx.SurfaceSignals, append(opts, graphqlx.WithDevDetails(a.Dev))...))

	// Guard after middleware, which is the order AdminSessionGuard runs in for the same reason - it reads the injected claims
	guarded := auth.ServiceSessionGuard(a.Revocations)(srv)
	return auth.ServiceMiddleware(signalsapi.Audience, a.JWTSecret)(guarded)
}
