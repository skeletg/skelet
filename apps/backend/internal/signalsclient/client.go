// Package signalsclient is core's plug into the Signals service - it holds the connection, mints the
// caller's token into request metadata, and hands out the six generated service clients
package signalsclient

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

// ErrUnavailable is a caller not reaching the service at all, as against the service answering "no"
var ErrUnavailable = errors.New("the Signals service is unavailable")

// Grants resolves a caller's Signals permissions, satisfied by *admin.Service
type Grants interface {
	Grants(ctx context.Context, adminID uuid.UUID) ([]auth.Grant, error)
}

type Client struct {
	conn   *grpc.ClientConn
	secret string
	ttl    time.Duration
	grants Grants

	Prism       signalsv1.PrismServiceClient
	Lens        signalsv1.LensServiceClient
	Synapse     signalsv1.SynapseServiceClient
	Segments    signalsv1.SegmentServiceClient
	Pulse       signalsv1.PulseServiceClient
	Destination signalsv1.DestinationServiceClient
}

// Dialling is lazy, so this never blocks on a service that is not up yet
func New(addr, jwtSecret string, ttl time.Duration, grants Grants) (*Client, error) {
	if addr == "" || jwtSecret == "" {
		return nil, nil
	}
	// In-cluster and behind the same trust boundary as the databases, so no TLS here
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: conn, secret: jwtSecret, ttl: ttl, grants: grants,
		Prism:       signalsv1.NewPrismServiceClient(conn),
		Lens:        signalsv1.NewLensServiceClient(conn),
		Synapse:     signalsv1.NewSynapseServiceClient(conn),
		Segments:    signalsv1.NewSegmentServiceClient(conn),
		Pulse:       signalsv1.NewPulseServiceClient(conn),
		Destination: signalsv1.NewDestinationServiceClient(conn),
	}, nil
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
