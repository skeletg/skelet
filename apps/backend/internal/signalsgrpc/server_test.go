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

// The behavioural-segment half of Synapse is not served yet, because its engine still lives in the
// resolver layer. Pinning that here means the day it moves, this test is what says so
// Every other Synapse and Lens method has to refuse an anonymous caller, same as Prism
func TestReadMethodsRefuseAnAnonymousCaller(t *testing.T) {
	conn := testConn(t)
	sy, lc := signalsv1.NewSynapseServiceClient(conn), signalsv1.NewLensServiceClient(conn)
	ctx := context.Background()
	id := uuid.New().String()

	for name, call := range map[string]func() error{
		"Synapse.GetSynapseOverview": func() error {
			_, e := sy.GetSynapseOverview(ctx, &signalsv1.GetSynapseOverviewRequest{ChannelId: id})
			return e
		},
		"Synapse.GetFunnel":     func() error { _, e := sy.GetFunnel(ctx, &signalsv1.GetFunnelRequest{ChannelId: id}); return e },
		"Synapse.GetFlow":       func() error { _, e := sy.GetFlow(ctx, &signalsv1.GetFlowRequest{ChannelId: id}); return e },
		"Synapse.GetRetention":  func() error { _, e := sy.GetRetention(ctx, &signalsv1.GetRetentionRequest{ChannelId: id}); return e },
		"Synapse.GetStickiness": func() error { _, e := sy.GetStickiness(ctx, &signalsv1.GetStickinessRequest{ChannelId: id}); return e },
		"Synapse.GetInsights":   func() error { _, e := sy.GetInsights(ctx, &signalsv1.GetInsightsRequest{ChannelId: id}); return e },
		"Synapse.GetCustomerProfile": func() error {
			_, e := sy.GetCustomerProfile(ctx, &signalsv1.GetCustomerProfileRequest{CustomerId: id})
			return e
		},
		"Synapse.GetVisitorProfile": func() error {
			_, e := sy.GetVisitorProfile(ctx, &signalsv1.GetVisitorProfileRequest{ChannelId: id, AnonId: "a"})
			return e
		},
		"Synapse.ListCustomerAudiences": func() error {
			_, e := sy.ListCustomerAudiences(ctx, &signalsv1.ListCustomerAudiencesRequest{CustomerId: id})
			return e
		},
		"Synapse.ListTrackedAttributes": func() error {
			_, e := sy.ListTrackedAttributes(ctx, &signalsv1.ListTrackedAttributesRequest{ChannelId: id})
			return e
		},
		"Synapse.ListAffinityValues": func() error {
			_, e := sy.ListAffinityValues(ctx, &signalsv1.ListAffinityValuesRequest{ChannelId: id, Attribute: "brand"})
			return e
		},
		"Synapse.ListBehavioralSegments": func() error {
			_, e := sy.ListBehavioralSegments(ctx, &signalsv1.ListBehavioralSegmentsRequest{ChannelId: id})
			return e
		},
		"Synapse.GetBehavioralSegment": func() error {
			_, e := sy.GetBehavioralSegment(ctx, &signalsv1.GetBehavioralSegmentRequest{Id: id})
			return e
		},
		"Synapse.PreviewBehavioralSegment": func() error {
			_, e := sy.PreviewBehavioralSegment(ctx, &signalsv1.PreviewBehavioralSegmentRequest{ChannelId: id})
			return e
		},
		"Synapse.SaveBehavioralSegment": func() error {
			_, e := sy.SaveBehavioralSegment(ctx, &signalsv1.SaveBehavioralSegmentRequest{ChannelId: id, Name: "n"})
			return e
		},
		"Synapse.RefreshBehavioralSegment": func() error {
			_, e := sy.RefreshBehavioralSegment(ctx, &signalsv1.RefreshBehavioralSegmentRequest{SegmentId: id})
			return e
		},
		"Synapse.SetSegmentLifecycleWindows": func() error {
			_, e := sy.SetSegmentLifecycleWindows(ctx, &signalsv1.SetSegmentLifecycleWindowsRequest{SegmentId: id})
			return e
		},
		"Lens.GetOverview":   func() error { _, e := lc.GetOverview(ctx, &signalsv1.GetOverviewRequest{ChannelId: id}); return e },
		"Lens.GetBotTraffic": func() error { _, e := lc.GetBotTraffic(ctx, &signalsv1.GetBotTrafficRequest{ChannelId: id}); return e },
		"Lens.ListPages":     func() error { _, e := lc.ListPages(ctx, &signalsv1.ListPagesRequest{ChannelId: id}); return e },
		"Lens.ListStates":    func() error { _, e := lc.ListStates(ctx, &signalsv1.ListStatesRequest{ChannelId: id}); return e },
		"Lens.GetPageStats":  func() error { _, e := lc.GetPageStats(ctx, &signalsv1.GetPageStatsRequest{ChannelId: id}); return e },
		"Lens.GetHeatmap":    func() error { _, e := lc.GetHeatmap(ctx, &signalsv1.GetHeatmapRequest{ChannelId: id}); return e },
		"Lens.ListSessions":  func() error { _, e := lc.ListSessions(ctx, &signalsv1.ListSessionsRequest{ChannelId: id}); return e },
		"Lens.ListCustomerSessions": func() error {
			_, e := lc.ListCustomerSessions(ctx, &signalsv1.ListCustomerSessionsRequest{CustomerId: id})
			return e
		},
		"Lens.ListVisitorSessions": func() error {
			_, e := lc.ListVisitorSessions(ctx, &signalsv1.ListVisitorSessionsRequest{AnonId: "a"})
			return e
		},
		"Lens.GetSessionSignals": func() error { _, e := lc.GetSessionSignals(ctx, &signalsv1.GetSessionSignalsRequest{Id: id}); return e },
		"Lens.GetFrustration": func() error {
			_, e := lc.GetFrustration(ctx, &signalsv1.GetFrustrationRequest{ChannelId: id})
			return e
		},
		"Lens.ListErrors": func() error { _, e := lc.ListErrors(ctx, &signalsv1.ListErrorsRequest{ChannelId: id}); return e },
		"Lens.GetPerformance": func() error {
			_, e := lc.GetPerformance(ctx, &signalsv1.GetPerformanceRequest{ChannelId: id})
			return e
		},
		"Lens.GetVitalsSummary": func() error {
			_, e := lc.GetVitalsSummary(ctx, &signalsv1.GetVitalsSummaryRequest{ChannelId: id})
			return e
		},
		"Lens.GetSearchAnalytics": func() error {
			_, e := lc.GetSearchAnalytics(ctx, &signalsv1.GetSearchAnalyticsRequest{ChannelId: id})
			return e
		},
	} {
		if got := status.Code(call()); got != codes.Unauthenticated {
			t.Errorf("%s answered an anonymous caller with %s, want Unauthenticated", name, got)
		}
	}
}

// The streaming path has its own interceptor chain, so it needs its own proof - and refusing an
// anonymous caller is not that proof, since a missing interceptor looks identical to no token
// What distinguishes them is a real token getting through, so both directions are asserted here
func TestTheStreamingPathRunsItsOwnAuth(t *testing.T) {
	c := signalsv1.NewLensServiceClient(testConn(t))

	drain := func(ctx context.Context) error {
		stream, err := c.GetSessionEvents(ctx, &signalsv1.GetSessionEventsRequest{Id: uuid.New().String()})
		if err != nil {
			return err
		}
		_, err = stream.Recv()
		return err
	}

	if got := status.Code(drain(context.Background())); got != codes.Unauthenticated {
		t.Errorf("the replay stream answered an anonymous caller with %s, want Unauthenticated", got)
	}
	// A valid token has to reach the handler - it then fails on the nil service, which is the proof it arrived
	if got := status.Code(drain(tokenCtx(t))); got == codes.Unauthenticated {
		t.Error("a valid token did not survive the stream interceptor")
	}
}
