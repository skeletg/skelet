// Package signalsgrpc serves the Signals suite over its wire contract. The handlers hold no logic of
// their own - they check the caller's permission, call the same services the GraphQL surface calls,
// and map the answer onto the proto
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

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/behavioralsegment"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/destination"
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

// Deps is what the suite's handlers read - the same services the GraphQL surface is built from
type Deps struct {
	Experiment  *experiment.Service
	Lens        *lens.Service
	Destination *destination.Service
	Segment     *segment.Service
	Analytics   *analytics.Recorder
	Behavioral  *behavioralsegment.Service
	// The suite's one commerce read - which attributes are flagged for affinity tracking
	Commerce *db.Queries
	// Where a customer's channel comes from, since this service has no customers table
	Roster    *signalsdb.Queries
	JWTSecret string
	// Dev turns on reflection, which lets grpcurl introspect a running server
	Dev bool
}

// NewServer wires the interceptors, the health service and every suite handler onto one gRPC server
func NewServer(d Deps) *grpc.Server {
	// Recovery outermost, so a panic in auth is caught too
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(recoverUnary(), auth.ServiceUnaryInterceptor(signalsapi.Audience, d.JWTSecret)),
		grpc.ChainStreamInterceptor(recoverStream(), auth.ServiceStreamInterceptor(signalsapi.Audience, d.JWTSecret)),
	)
	signalsv1.RegisterPrismServiceServer(srv, &prismServer{svc: d.Experiment})
	signalsv1.RegisterLensServiceServer(srv, &lensServer{svc: d.Lens})
	signalsv1.RegisterDestinationServiceServer(srv, &destinationServer{svc: d.Destination})
	signalsv1.RegisterSegmentServiceServer(srv, &segmentServer{svc: d.Segment})
	signalsv1.RegisterPulseServiceServer(srv, &pulseServer{rec: d.Analytics})
	signalsv1.RegisterIngestServiceServer(srv, &ingestServer{rec: d.Analytics})
	signalsv1.RegisterSynapseServiceServer(srv, &synapseServer{
		rec: d.Analytics, segments: d.Segment, behavioral: d.Behavioral, commerce: d.Commerce, roster: d.Roster,
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
