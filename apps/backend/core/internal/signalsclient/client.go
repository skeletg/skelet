// Package signalsclient is core's plug into the Signals service - it holds the connection, mints the
// caller's token into request metadata, and hands out the generated service clients
package signalsclient

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// ErrUnavailable is a caller not reaching the service at all, as against the service answering "no"
var ErrUnavailable = errors.New("the Signals service is unavailable")

// Grants resolves a caller's Signals permissions, satisfied by *admin.Service
type Grants interface {
	Grants(ctx context.Context, adminID uuid.UUID) ([]auth.Grant, error)
}

type adminStanding interface {
	Standing(ctx context.Context, adminID uuid.UUID) (superuser, active bool, err error)
}

var ErrAdminInactive = errors.New("the admin this work acts for is no longer active")

type Client struct {
	conn   *grpc.ClientConn
	secret string
	ttl    time.Duration
	grants Grants

	segments segmentGuard

	Ingest       signalsv1.IngestServiceClient
	Prism        signalsv1.PrismServiceClient
	Lens         signalsv1.LensServiceClient
	Synapse      signalsv1.SynapseServiceClient
	Segments     signalsv1.SegmentServiceClient
	Pulse        signalsv1.PulseServiceClient
	Destination  signalsv1.DestinationServiceClient
	Maintenance  signalsv1.MaintenanceServiceClient
	Settings     signalsv1.SettingsServiceClient
	Suppressions signalsv1.SuppressionServiceClient
	// Pulse - the flow composer, the campaign composer and the email builder behind them
	Flows     signalsv1.FlowServiceClient
	Campaigns signalsv1.CampaignServiceClient
	Builder   signalsv1.EmailBuilderServiceClient
}

// Dialling is lazy, so this never blocks on a service that is not up yet
// serviceSecret signs the tokens this client mints - not the session secret, which the storefront holds too
func New(addr, serviceSecret string, ttl time.Duration, grants Grants) (*Client, error) {
	if addr == "" || serviceSecret == "" {
		return nil, nil
	}
	// passthrough, so the hostname resolves on every reconnect - otherwise a restart of the suite leaves this client on an empty address list
	// In-cluster and behind the same trust boundary as the databases, so no TLS here
	conn, err := grpc.NewClient("passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(deadlineUnary()),
		grpc.WithChainStreamInterceptor(deadlineStream()),
		otelx.GRPCClient(),
	)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: conn, secret: serviceSecret, ttl: ttl, grants: grants,
		segments:     newSegmentGuard(),
		Ingest:       signalsv1.NewIngestServiceClient(conn),
		Prism:        signalsv1.NewPrismServiceClient(conn),
		Lens:         signalsv1.NewLensServiceClient(conn),
		Synapse:      signalsv1.NewSynapseServiceClient(conn),
		Segments:     signalsv1.NewSegmentServiceClient(conn),
		Pulse:        signalsv1.NewPulseServiceClient(conn),
		Destination:  signalsv1.NewDestinationServiceClient(conn),
		Maintenance:  signalsv1.NewMaintenanceServiceClient(conn),
		Settings:     signalsv1.NewSettingsServiceClient(conn),
		Suppressions: signalsv1.NewSuppressionServiceClient(conn),
		Flows:        signalsv1.NewFlowServiceClient(conn),
		Campaigns:    signalsv1.NewCampaignServiceClient(conn),
		Builder:      signalsv1.NewEmailBuilderServiceClient(conn),
	}, nil
}

// The deadline a call gets when its caller set none - a hung suite must not hang a password reset, and a dump
// or a restore needs its room. A caller's own deadline always wins
const (
	defaultCallTimeout     = 10 * time.Second
	maintenanceCallTimeout = 30 * time.Minute
)

func callTimeout(method string) time.Duration {
	if strings.HasPrefix(method, "/"+signalsv1.MaintenanceService_ServiceDesc.ServiceName+"/") {
		return maintenanceCallTimeout
	}
	return defaultCallTimeout
}

func deadlineUnary() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if _, set := ctx.Deadline(); !set {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, callTimeout(method))
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// Streams are the bulk paths, so they get the long deadline - released when the stream ends, not when this returns
func deadlineStream() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if _, set := ctx.Deadline(); set {
			return streamer(ctx, desc, cc, method, opts...)
		}
		ctx, cancel := context.WithTimeout(ctx, maintenanceCallTimeout)
		cs, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			cancel()
			return nil, err
		}
		return &deadlinedStream{ClientStream: cs, cancel: cancel, serverStreams: desc.ServerStreams}, nil
	}
}

type deadlinedStream struct {
	grpc.ClientStream
	cancel        context.CancelFunc
	serverStreams bool
}

// A server stream ends on the first Recv error (io.EOF included) - a client stream's one response is its end
func (s *deadlinedStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil || !s.serverStreams {
		s.cancel()
	}
	return err
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Authed attaches the caller's audience-scoped token to the outgoing request
func (c *Client) Authed(ctx context.Context) (context.Context, error) {
	if c == nil {
		return nil, ErrUnavailable
	}
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	sc := auth.ServiceClaims{
		AdminID:     claims.AdminID,
		SessionID:   claims.SessionID,
		Email:       claims.Email,
		IsSuperuser: claims.IsSuperuser,
	}
	// A superuser bypasses every check before a grant is read, so their token stays empty
	if !claims.IsSuperuser && c.grants != nil {
		grants, err := c.grants.Grants(ctx, claims.AdminID)
		if err != nil {
			return nil, err
		}
		sc.Grants = auth.FilterGrants(grants, signalsapi.Slugs)
	}
	token, err := auth.GenerateServiceToken(sc, signalsapi.Audience, c.secret, c.ttl)
	if err != nil {
		return nil, err
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token), nil
}

// System attaches core's own token, for the writes the event bus raises with no admin to act for
// It carries no grants, so it authorizes the ingest methods and nothing else
func (c *Client) System(ctx context.Context) (context.Context, error) {
	if c == nil {
		return nil, ErrUnavailable
	}
	token, err := auth.GenerateServiceToken(auth.ServiceClaims{System: true}, signalsapi.Audience, c.secret, c.ttl)
	if err != nil {
		return nil, err
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token), nil
}

// UNAVAILABLE and DEADLINE_EXCEEDED are the transport failing - everything else is an answer
func Unreachable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrUnavailable) {
		return true
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}

// AsAdmin mints a token carrying a named admin's grants, for work core does on their behalf once the request that asked for it has gone
// The suite still decides what those grants open
func (c *Client) AsAdmin(ctx context.Context, adminID uuid.UUID) (context.Context, error) {
	if c == nil {
		return nil, ErrUnavailable
	}
	sc := auth.ServiceClaims{AdminID: adminID}
	if st, ok := c.grants.(adminStanding); ok {
		superuser, active, err := st.Standing(ctx, adminID)
		if err != nil {
			return nil, err
		}
		if !active {
			return nil, ErrAdminInactive
		}
		sc.IsSuperuser = superuser
	}
	if c.grants != nil && !sc.IsSuperuser {
		grants, err := c.grants.Grants(ctx, adminID)
		if err != nil {
			return nil, err
		}
		sc.Grants = auth.FilterGrants(grants, signalsapi.Slugs)
	}
	token, err := auth.GenerateServiceToken(sc, signalsapi.Audience, c.secret, c.ttl)
	if err != nil {
		return nil, err
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token), nil
}

// AsCustomer mints a token naming one shopper, for the storefront paths that price a cart or render a
// gated block - there is no admin behind those, and the suite answers about that customer alone
func (c *Client) AsCustomer(ctx context.Context, customerID uuid.UUID) (context.Context, error) {
	if c == nil {
		return nil, ErrUnavailable
	}
	token, err := auth.GenerateServiceToken(auth.ServiceClaims{CustomerID: customerID}, signalsapi.Audience, c.secret, c.ttl)
	if err != nil {
		return nil, err
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token), nil
}
