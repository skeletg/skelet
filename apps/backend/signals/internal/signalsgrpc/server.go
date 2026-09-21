// Package signalsgrpc serves the Signals suite over its wire contract - the handlers check the permission, call the service, map the proto
package signalsgrpc

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics/spinequeue"
	"github.com/skeletg/skelet/apps/backend/signals/internal/behavioralsegment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/destination"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/engagement"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/flow"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/signals/internal/signalsboot"
	"github.com/skeletg/skelet/apps/backend/signals/internal/suppression"
)

// Deps is what the suite's handlers read - the same services the GraphQL surface is built from
type Deps struct {
	Experiment  *experiment.Service
	Lens        *lens.Service
	Destination *destination.Service
	Segment     *segment.Service
	Analytics   *analytics.Recorder
	Behavioral  *behavioralsegment.Service
	Channels    *analytics.ChannelRoster
	// The suite's own upkeep, driven from core's scheduler
	Settings signalsboot.Settings
	Queue    *spinequeue.Queue // optional - nil when events go straight to the engine
	// The suite's own settings store, which core administers over the contract
	SettingsRepo config.Repository
	// The send log and the suppression list - the engines here and core's transactional sends write and read them
	SendLog     *sendlog.Store
	Suppression *suppression.Service
	// Pulse: the flow engine, the campaign pipeline and the email builder they render from - nil in a
	// test server without them, and the services then go unregistered
	Flows     *flow.Service
	Campaigns *campaign.Service
	Builder   *emailtemplate.Service
	// The rosters as the Pulse surfaces read them - the shop a send goes out as, and the person it reaches
	Stores   emailtemplate.StoreReader
	Contacts PreviewContacts
	// Where a customer's channel comes from, since this service has no customers table
	Roster *signalsdb.Queries
	// Verifies the tokens core signs its calls with - the contract's own secret, not the session one
	ServiceSecret string
	// Killed admin sessions, so a token minted before a logout is refused - nil skips the check
	Revocations auth.Revocations
	// Dev turns on reflection, which lets grpcurl introspect a running server
	Dev bool
}

// PreviewContacts is the roster read a preview rendered as a real customer makes
type PreviewContacts = previewContacts

// PreviewContact is what it answers
type PreviewContact = previewContact

// NewServer wires the interceptors, the health service and every suite handler onto one gRPC server
func NewServer(d Deps) *grpc.Server {
	// Recovery outermost, so a panic in auth is caught too - the stats handler continues the trace core started
	srv := grpc.NewServer(
		otelx.GRPCServer(),
		grpc.ChainUnaryInterceptor(recoverUnary(), auth.ServiceUnaryInterceptor(signalsapi.Audience, d.ServiceSecret, d.Revocations)),
		grpc.ChainStreamInterceptor(recoverStream(), auth.ServiceStreamInterceptor(signalsapi.Audience, d.ServiceSecret, d.Revocations)),
	)
	signalsv1.RegisterPrismServiceServer(srv, &prismServer{svc: d.Experiment})
	signalsv1.RegisterLensServiceServer(srv, &lensServer{svc: d.Lens})
	signalsv1.RegisterDestinationServiceServer(srv, &destinationServer{svc: d.Destination})
	signalsv1.RegisterSegmentServiceServer(srv, &segmentServer{svc: d.Segment})
	signalsv1.RegisterPulseServiceServer(srv, &pulseServer{rec: d.Analytics, sends: d.SendLog, roster: rosterOrNil(d.Roster)})
	signalsv1.RegisterIngestServiceServer(srv, &ingestServer{
		rec: d.Analytics, channels: d.Channels, sends: d.SendLog, track: engagement.New(d.Analytics, d.SendLog),
	})
	signalsv1.RegisterSettingsServiceServer(srv, &settingsServer{repo: d.SettingsRepo})
	maint := &maintenanceServer{
		rec: d.Analytics, lens: d.Lens, destination: d.Destination, settings: d.Settings,
		segments: d.Segment, behavioral: d.Behavioral, queue: queueOrNil(d.Queue), sends: sendLogOrNil(d.SendLog),
	}
	if d.Flows != nil {
		maint.flows = d.Flows
		signalsv1.RegisterFlowServiceServer(srv, &flowServer{svc: d.Flows, roster: rosterOrNil(d.Roster)})
	}
	if d.Campaigns != nil {
		maint.campaigns = d.Campaigns
		signalsv1.RegisterCampaignServiceServer(srv, &campaignServer{svc: d.Campaigns})
	}
	if d.Builder != nil {
		signalsv1.RegisterEmailBuilderServiceServer(srv, &builderServer{svc: d.Builder, stores: d.Stores, contacts: d.Contacts})
	}
	signalsv1.RegisterMaintenanceServiceServer(srv, maint)
	signalsv1.RegisterSuppressionServiceServer(srv, &suppressionServer{svc: d.Suppression})
	signalsv1.RegisterSynapseServiceServer(srv, &synapseServer{
		rec: d.Analytics, segments: d.Segment, behavioral: d.Behavioral, roster: d.Roster,
	})

	// Load balancers and orchestrators watch this rather than opening a real call
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, hs)
	if d.Dev {
		reflection.Register(srv)
	}
	return srv
}

// ---- shared helpers ----

// The permission a handler needs, refused as PERMISSION_DENIED so a caller can tell it apart from a
// server fault - the interceptor is non-blocking, so this is where access is actually decided
func requirePermission(ctx context.Context, perm string) error {
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), perm); err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			return status.Error(codes.Unauthenticated, "no valid service token")
		}
		return status.Errorf(codes.PermissionDenied, "%s is required", perm)
	}
	return nil
}

// Every channel-scoped read shares this gate: the permission, then the same permission on the channel
// actually asked for, so a grant for one store cannot read another's
func channelScope(ctx context.Context, perm, channelID string) (uuid.UUID, error) {
	if err := requirePermission(ctx, perm); err != nil {
		return uuid.Nil, err
	}
	cid, err := parseID("channel_id", channelID)
	if err != nil {
		return uuid.Nil, err
	}
	if err := requireChannel(ctx, perm, cid); err != nil {
		return uuid.Nil, err
	}
	return cid, nil
}

// Core's own automation runs with no admin behind it, so the named methods take the system token as well as the permission
func requirePermissionOrSystem(ctx context.Context, perms ...string) error {
	if auth.IsSystemCaller(ctx) {
		return nil
	}
	for _, p := range perms {
		if err := requirePermission(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func requireChannel(ctx context.Context, perm string, channelID uuid.UUID) error {
	if err := auth.RequireChannelPermission(ctx, auth.GrantCheckerFrom(ctx), perm, channelID, nil); err != nil {
		return status.Errorf(codes.PermissionDenied, "%s is required on that channel", perm)
	}
	return nil
}

func invalidArgument(format string, a ...any) error {
	return status.Errorf(codes.InvalidArgument, format, a...)
}

func parseID(field, raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, status.Errorf(codes.InvalidArgument, "%s is not a valid id", field)
	}
	return id, nil
}

// A window the caller left at zero takes the server's default, and one past the ceiling is clamped
// rather than refused, since an over-long window is a slow query rather than a wrong one
func windowDays(requested, fallback int32) int32 {
	if requested <= 0 {
		return fallback
	}
	if requested > 365 {
		return 365
	}
	return requested
}

// A zero time marshals as an unset field rather than the zero instant, which reads as year 1
// A blank or unparseable id is the nil uuid, which the recorder reads as "no channel"
func uuidOrNil(raw string) uuid.UUID {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// An absent optional id stays absent rather than becoming the nil uuid
func optionalUUID(raw *string) *uuid.UUID {
	if raw == nil || *raw == "" {
		return nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil
	}
	return &id
}

func ts(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// A typed nil would satisfy the interface and then panic on the first call
func queueOrNil(q *spinequeue.Queue) queueStats {
	if q == nil {
		return nil
	}
	return q
}

// A nil *Store must not become a non-nil interface, or every call would dereference it
func sendLogOrNil(s *sendlog.Store) sendLogMaintainer {
	if s == nil {
		return nil
	}
	return s
}

// The same typed-nil guard for the roster handle
func rosterOrNil(q *signalsdb.Queries) rosterChannels {
	if q == nil {
		return nil
	}
	return q
}
