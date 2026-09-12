package signalsgrpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

const testSecret = "test-secret-for-the-service-token"

// Every service is left nil on purpose - permission and argument checks have to happen before a
// handler touches one, so a nil service is what proves the order
func testConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := NewServer(Deps{JWTSecret: testSecret})
	go func() { _ = srv.Serve(lis) }()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(); srv.Stop() })
	return conn
}

// A token carrying exactly the given grants, or a superuser's when none are named
func tokenCtx(t *testing.T, grants ...auth.Grant) context.Context {
	t.Helper()
	claims := auth.ServiceClaims{AdminID: uuid.New(), Email: "a@b.c", IsSuperuser: len(grants) == 0, Grants: grants}
	tok, err := auth.GenerateServiceToken(claims, signalsapi.Audience, testSecret, time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+tok)
}

func TestHealthReportsServing(t *testing.T) {
	res, err := healthpb.NewHealthClient(testConn(t)).Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if res.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("status is %s, want SERVING", res.GetStatus())
	}
}

// The interceptor is non-blocking by design, so every handler has to refuse on its own - a method
// that forgot would answer an anonymous caller
func TestEveryPrismMethodRefusesAnAnonymousCaller(t *testing.T) {
	c := signalsv1.NewPrismServiceClient(testConn(t))
	ctx := context.Background()
	id, cid := uuid.New().String(), uuid.New().String()

	for name, call := range map[string]func() error{
		"ListExperiments": func() error {
			_, e := c.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: cid})
			return e
		},
		"GetExperiment": func() error { _, e := c.GetExperiment(ctx, &signalsv1.GetExperimentRequest{Id: id}); return e },
		"GetExperimentResults": func() error {
			_, e := c.GetExperimentResults(ctx, &signalsv1.GetExperimentResultsRequest{Id: id})
			return e
		},
		"GetExperimentsOverview": func() error {
			_, e := c.GetExperimentsOverview(ctx, &signalsv1.GetExperimentsOverviewRequest{ChannelId: cid})
			return e
		},
		"ListGoalEvents": func() error {
			_, e := c.ListGoalEvents(ctx, &signalsv1.ListGoalEventsRequest{ChannelId: cid})
			return e
		},
		"CreateExperiment": func() error { _, e := c.CreateExperiment(ctx, &signalsv1.CreateExperimentRequest{}); return e },
		"UpdateExperiment": func() error { _, e := c.UpdateExperiment(ctx, &signalsv1.UpdateExperimentRequest{Id: id}); return e },
		"DeleteExperiment": func() error { _, e := c.DeleteExperiment(ctx, &signalsv1.DeleteExperimentRequest{Id: id}); return e },
	} {
		if got := status.Code(call()); got != codes.Unauthenticated {
			t.Errorf("%s answered an anonymous caller with %s, want Unauthenticated", name, got)
		}
	}
}

// view is not manage - a token that can read experiments must not be able to write them
func TestWritesNeedTheManageGrant(t *testing.T) {
	c := signalsv1.NewPrismServiceClient(testConn(t))
	ctx := tokenCtx(t, auth.Grant{Slug: "experiments.view"})

	if got := status.Code(mustErr(c.CreateExperiment(ctx, &signalsv1.CreateExperimentRequest{}))); got != codes.PermissionDenied {
		t.Errorf("create with view-only answered %s, want PermissionDenied", got)
	}
	if got := status.Code(mustErr(c.DeleteExperiment(ctx, &signalsv1.DeleteExperimentRequest{Id: uuid.New().String()}))); got != codes.PermissionDenied {
		t.Errorf("delete with view-only answered %s, want PermissionDenied", got)
	}
	// The same token passes the read gate and fails later, on the nil service - which is the proof it got past
	if got := status.Code(mustErr(c.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: uuid.New().String()}))); got == codes.PermissionDenied {
		t.Error("a view grant was refused on a read")
	}
}

// A malformed id is the caller's mistake, not a server fault, and it has to be caught before any lookup
func TestMalformedIdsAreInvalidArgument(t *testing.T) {
	c := signalsv1.NewPrismServiceClient(testConn(t))
	ctx := tokenCtx(t)

	if got := status.Code(mustErr(c.GetExperiment(ctx, &signalsv1.GetExperimentRequest{Id: "not-a-uuid"}))); got != codes.InvalidArgument {
		t.Errorf("bad id answered %s, want InvalidArgument", got)
	}
	if got := status.Code(mustErr(c.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: "nope"}))); got != codes.InvalidArgument {
		t.Errorf("bad channel id answered %s, want InvalidArgument", got)
	}
	if got := status.Code(mustErr(c.CreateExperiment(ctx, &signalsv1.CreateExperimentRequest{}))); got != codes.InvalidArgument {
		t.Errorf("a create with no body answered %s, want InvalidArgument", got)
	}
}

func TestWindowDaysClampsRatherThanRefuses(t *testing.T) {
	for _, tc := range []struct{ in, want int32 }{{0, 30}, {-5, 30}, {7, 7}, {365, 365}, {10_000, 365}} {
		if got := windowDays(tc.in, 30); got != tc.want {
			t.Errorf("windowDays(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// An absent time has to marshal as an unset field, since the zero instant reads as year 1 on the wire
func TestZeroTimesMarshalAsUnset(t *testing.T) {
	if ts(time.Time{}) != nil {
		t.Error("the zero time should marshal as unset")
	}
	now := time.Now()
	if got := ts(now); got == nil || !got.AsTime().Equal(now.UTC().Truncate(time.Nanosecond)) {
		t.Errorf("a real time did not survive the round trip: %v", got)
	}
}

func mustErr[T any](_ T, err error) error { return err }

// A nil service is a panic waiting to happen, and it is the cheapest way to prove one method's panic
// answers INTERNAL and leaves the connection usable for the next call
func TestAPanicAnswersInternalAndKeepsServing(t *testing.T) {
	conn := testConn(t)
	c := signalsv1.NewPrismServiceClient(conn)
	ctx := tokenCtx(t)

	if got := status.Code(mustErr(c.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: uuid.New().String()}))); got != codes.Internal {
		t.Errorf("a panicking handler answered %s, want Internal", got)
	}
	// The panic value itself must not leak, since it can carry whatever the handler was holding
	if msg := status.Convert(mustErr(c.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: uuid.New().String()}))).Message(); msg != "internal error" {
		t.Errorf("the panic leaked into the message: %q", msg)
	}
	// The server is still up, which is the whole point
	res, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil || res.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("the server did not survive the panic: %v %v", res.GetStatus(), err)
	}
}
