package signalsgrpc

import (
	"context"
	"errors"
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

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

const testSecret = "test-secret-for-the-service-token"

// Every service is left nil on purpose - permission and argument checks have to happen before a
// handler touches one, so a nil service is what proves the order
func testConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	return testConnWith(t, Deps{ServiceSecret: testSecret, Channels: analytics.NewChannelRoster(nil)})
}

func testConnWith(t *testing.T, deps Deps) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := NewServer(deps)
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

// The behavioural-segment half of Synapse is not served yet, because its engine still lives in the resolver layer. Pinning that here means the day it moves, this test is what says so Every other Synapse and Lens method has to refuse an anonymous caller, same as Prism
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

// The streaming path has its own interceptor chain, so it needs its own proof - and refusing an anonymous caller is not that proof, since a missing interceptor looks identical to no token What distinguishes them is a real token getting through, so both directions are asserted here
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

// A system token is core writing as itself, and it must buy nothing else on this server
func systemCtx(t *testing.T) context.Context {
	t.Helper()
	tok, err := auth.GenerateServiceToken(auth.ServiceClaims{System: true}, signalsapi.Audience, testSecret, time.Minute)
	if err != nil {
		t.Fatalf("mint system: %v", err)
	}
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+tok)
}

func TestASystemTokenCarriesNoAuthorityOfItsOwn(t *testing.T) {
	if _, err := auth.GenerateServiceToken(
		auth.ServiceClaims{System: true, IsSuperuser: true}, signalsapi.Audience, testSecret, time.Minute,
	); err == nil {
		t.Error("a superuser system token was minted, which would open every read to the ingest credential")
	}
	if _, err := auth.GenerateServiceToken(
		auth.ServiceClaims{System: true, Grants: []auth.Grant{{Slug: "lens.view"}}}, signalsapi.Audience, testSecret, time.Minute,
	); err == nil {
		t.Error("a granted system token was minted, which would let the ingest credential read")
	}
}

// The two principals are disjoint: the writes take a system token, every read takes an admin one
func TestTheIngestMethodsRefuseAnAdminToken(t *testing.T) {
	c := signalsv1.NewIngestServiceClient(testConn(t))
	admin := tokenCtx(t) // a superuser, so nothing but the principal check can be refusing it
	id := uuid.New().String()

	for name, call := range map[string]func(context.Context) error{
		"RecordActivity": func(ctx context.Context) error {
			_, e := c.RecordActivity(ctx, &signalsv1.RecordActivityRequest{EventName: "x", ChannelId: id})
			return e
		},
		"RecordConversion": func(ctx context.Context) error {
			_, e := c.RecordConversion(ctx, &signalsv1.RecordConversionRequest{CustomerId: id, OrderId: id})
			return e
		},
		"RecordEmailSent": func(ctx context.Context) error {
			_, e := c.RecordEmailSent(ctx, &signalsv1.RecordEmailSentRequest{Send: &signalsv1.EmailSendRef{SendId: id}})
			return e
		},
		"RecordEmailOpen": func(ctx context.Context) error {
			_, e := c.RecordEmailOpen(ctx, &signalsv1.RecordEmailOpenRequest{Send: &signalsv1.EmailSendRef{SendId: id}})
			return e
		},
		"RecordEmailClick": func(ctx context.Context) error {
			_, e := c.RecordEmailClick(ctx, &signalsv1.RecordEmailClickRequest{Send: &signalsv1.EmailSendRef{SendId: id}})
			return e
		},
	} {
		if got := status.Code(call(admin)); got != codes.PermissionDenied {
			t.Errorf("%s answered a superuser admin token with %s, want PermissionDenied", name, got)
		}
		if got := status.Code(call(context.Background())); got != codes.PermissionDenied {
			t.Errorf("%s answered an anonymous caller with %s, want PermissionDenied", name, got)
		}
	}
}

// The other direction of the same gate - refusing everyone would pass the test above while breaking every write
// Every service here is nil, so getting past the principal check is what reaching the handler looks like
func TestASystemTokenAuthorizesTheIngestMethods(t *testing.T) {
	c := signalsv1.NewIngestServiceClient(testConn(t))
	sys := systemCtx(t)
	id := uuid.New().String()

	for name, call := range map[string]func() error{
		"RecordActivity": func() error {
			_, e := c.RecordActivity(sys, &signalsv1.RecordActivityRequest{EventName: "x", ChannelId: id})
			return e
		},
		"RecordConversion": func() error {
			_, e := c.RecordConversion(sys, &signalsv1.RecordConversionRequest{CustomerId: id, OrderId: id})
			return e
		},
		"RecordEmailSent": func() error {
			_, e := c.RecordEmailSent(sys, &signalsv1.RecordEmailSentRequest{Send: &signalsv1.EmailSendRef{SendId: id}})
			return e
		},
		"RecordEmailOpen": func() error {
			_, e := c.RecordEmailOpen(sys, &signalsv1.RecordEmailOpenRequest{Send: &signalsv1.EmailSendRef{SendId: id}})
			return e
		},
		"RecordEmailClick": func() error {
			_, e := c.RecordEmailClick(sys, &signalsv1.RecordEmailClickRequest{Send: &signalsv1.EmailSendRef{SendId: id}})
			return e
		},
		"SyncChannels": func() error {
			_, e := c.SyncChannels(sys, &signalsv1.SyncChannelsRequest{
				Channels: []*signalsv1.ChannelEntry{{ChannelId: id}},
			})
			return e
		},
	} {
		switch got := status.Code(call()); got {
		case codes.PermissionDenied, codes.Unauthenticated:
			t.Errorf("%s refused core's own system token with %s, so no spine write would land", name, got)
		}
	}
}

// And the other way: a system token must not reach a read, whatever the service
func TestASystemTokenCannotRead(t *testing.T) {
	conn := testConn(t)
	sys := systemCtx(t)
	id := uuid.New().String()

	for name, call := range map[string]func() error{
		"Lens.GetOverview": func() error {
			_, e := signalsv1.NewLensServiceClient(conn).GetOverview(sys, &signalsv1.GetOverviewRequest{ChannelId: id})
			return e
		},
		"Synapse.GetSynapseOverview": func() error {
			_, e := signalsv1.NewSynapseServiceClient(conn).GetSynapseOverview(sys, &signalsv1.GetSynapseOverviewRequest{ChannelId: id})
			return e
		},
		"Prism.ListExperiments": func() error {
			_, e := signalsv1.NewPrismServiceClient(conn).ListExperiments(sys, &signalsv1.ListExperimentsRequest{ChannelId: id})
			return e
		},
		"Segment.ListSegments": func() error {
			_, e := signalsv1.NewSegmentServiceClient(conn).ListSegments(sys, &signalsv1.ListSegmentsRequest{})
			return e
		},
	} {
		switch got := status.Code(call()); got {
		case codes.Unauthenticated, codes.PermissionDenied:
		default:
			t.Errorf("%s let a system token through with %s", name, got)
		}
	}
}

// A token naming one customer is the storefront's whole authority - a cart being priced has no admin
// behind it, and a token that could ask about anyone else would be a customer-enumeration hole
func TestACustomerTokenReadsOnlyItsOwnCustomer(t *testing.T) {
	c := signalsv1.NewSegmentServiceClient(testConn(t))
	mine, theirs := uuid.New(), uuid.New()
	ctx := customerCtx(t, mine)

	if got := status.Code(callSlugs(c, ctx, theirs)); got != codes.PermissionDenied {
		t.Errorf("asking about another customer = %s, want PermissionDenied", got)
	}
	switch got := status.Code(callSlugs(c, ctx, mine)); got {
	case codes.PermissionDenied, codes.Unauthenticated:
		t.Errorf("asking about its own customer = %s, so a segment-gated cart would never match", got)
	}
}

// And it is authority over that one read, not a way onto the rest of the surface
func TestACustomerTokenCannotReachTheAdminSurface(t *testing.T) {
	conn := testConn(t)
	ctx := customerCtx(t, uuid.New())
	id := uuid.New().String()

	for name, call := range map[string]func() error{
		"Segment.ListSegments": func() error {
			_, e := signalsv1.NewSegmentServiceClient(conn).ListSegments(ctx, &signalsv1.ListSegmentsRequest{})
			return e
		},
		"Segment.ListMembers": func() error {
			_, e := signalsv1.NewSegmentServiceClient(conn).ListMembers(ctx, &signalsv1.ListMembersRequest{SegmentId: id})
			return e
		},
		"Lens.GetOverview": func() error {
			_, e := signalsv1.NewLensServiceClient(conn).GetOverview(ctx, &signalsv1.GetOverviewRequest{ChannelId: id})
			return e
		},
		"Synapse.GetCustomerProfile": func() error {
			_, e := signalsv1.NewSynapseServiceClient(conn).GetCustomerProfile(ctx, &signalsv1.GetCustomerProfileRequest{CustomerId: id})
			return e
		},
	} {
		switch got := status.Code(call()); got {
		case codes.PermissionDenied, codes.Unauthenticated:
		default:
			t.Errorf("%s answered a customer token with %s, want it refused", name, got)
		}
	}
}

// Core's own automation carries no admin, so these two take the system token - and only these two
func TestASystemTokenReachesTheAutomationMethodsOnly(t *testing.T) {
	c := signalsv1.NewSegmentServiceClient(testConn(t))
	sys := systemCtx(t)
	id := uuid.New().String()

	for name, call := range map[string]func() error{
		"ListMembers": func() error {
			_, e := c.ListMembers(sys, &signalsv1.ListMembersRequest{SegmentId: id})
			return e
		},
		"AssignCustomers": func() error {
			_, e := c.AssignCustomers(sys, &signalsv1.AssignCustomersRequest{SegmentId: id, CustomerIds: []string{uuid.NewString()}})
			return e
		},
	} {
		switch got := status.Code(call()); got {
		case codes.PermissionDenied, codes.Unauthenticated:
			t.Errorf("%s refused core's own system token with %s, so a campaign send or a flow node would do nothing", name, got)
		}
	}
	if got := status.Code(callSlugs(c, sys, uuid.New())); got != codes.PermissionDenied {
		t.Errorf("ListCustomerSegments took a system token (%s) - it is the customer principal's", got)
	}
}

func customerCtx(t *testing.T, customerID uuid.UUID) context.Context {
	t.Helper()
	tok, err := auth.GenerateServiceToken(auth.ServiceClaims{CustomerID: customerID}, signalsapi.Audience, testSecret, time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+tok)
}

func callSlugs(c signalsv1.SegmentServiceClient, ctx context.Context, id uuid.UUID) error {
	_, err := c.ListCustomerSegments(ctx, &signalsv1.ListCustomerSegmentsRequest{CustomerId: id.String()})
	return err
}

// Core's scheduler drives the suite's upkeep, so these take the system token - a scheduled run has
// no admin behind it, and refusing one would age out nothing and forward nothing, silently
func TestASystemTokenAuthorizesTheMaintenanceMethods(t *testing.T) {
	c := signalsv1.NewMaintenanceServiceClient(testConn(t))
	sys := systemCtx(t)

	for name, call := range map[string]func() error{
		"PruneSpine":          func() error { _, e := c.PruneSpine(sys, &signalsv1.PruneSpineRequest{}); return e },
		"PruneLens":           func() error { _, e := c.PruneLens(sys, &signalsv1.PruneLensRequest{}); return e },
		"ComputeTraits":       func() error { _, e := c.ComputeTraits(sys, &signalsv1.ComputeTraitsRequest{}); return e },
		"ForwardDestinations": func() error { _, e := c.ForwardDestinations(sys, &signalsv1.ForwardDestinationsRequest{}); return e },
		"GetRetentionAction":  func() error { _, e := c.GetRetentionAction(sys, &signalsv1.GetRetentionActionRequest{}); return e },
		"GetCustomerTraits": func() error {
			_, e := c.GetCustomerTraits(sys, &signalsv1.GetCustomerTraitsRequest{CustomerId: uuid.NewString()})
			return e
		},
	} {
		if got := status.Code(call()); got == codes.PermissionDenied || got == codes.Unauthenticated {
			t.Errorf("%s refused core's own system token with %s", name, got)
		}
	}
}

// And an admin token reaches none of them - upkeep is core's to drive, not a dashboard button, and a
// method that forgot its gate would be an unguarded door onto the suite's data
func TestAnAdminTokenCannotDriveMaintenance(t *testing.T) {
	c := signalsv1.NewMaintenanceServiceClient(testConn(t))
	adm := tokenCtx(t)

	for name, call := range map[string]func() error{
		"PruneSpine":           func() error { _, e := c.PruneSpine(adm, &signalsv1.PruneSpineRequest{}); return e },
		"PruneLens":            func() error { _, e := c.PruneLens(adm, &signalsv1.PruneLensRequest{}); return e },
		"ComputeTraits":        func() error { _, e := c.ComputeTraits(adm, &signalsv1.ComputeTraitsRequest{}); return e },
		"ForwardDestinations":  func() error { _, e := c.ForwardDestinations(adm, &signalsv1.ForwardDestinationsRequest{}); return e },
		"GetRetentionAction":   func() error { _, e := c.GetRetentionAction(adm, &signalsv1.GetRetentionActionRequest{}); return e },
		"GetHealth":            func() error { _, e := c.GetHealth(adm, &signalsv1.GetHealthRequest{}); return e },
		"ListBrowseAbandoners": func() error { _, e := c.ListBrowseAbandoners(adm, &signalsv1.ListBrowseAbandonersRequest{}); return e },
		"MarkBrowseNotified":   func() error { _, e := c.MarkBrowseNotified(adm, &signalsv1.MarkBrowseNotifiedRequest{}); return e },
		"RefreshDynamicSegments": func() error {
			_, e := c.RefreshDynamicSegments(adm, &signalsv1.RefreshDynamicSegmentsRequest{})
			return e
		},
		"RefreshBehavioralSegments": func() error {
			_, e := c.RefreshBehavioralSegments(adm, &signalsv1.RefreshBehavioralSegmentsRequest{})
			return e
		},
		"GetCustomerTraits": func() error {
			_, e := c.GetCustomerTraits(adm, &signalsv1.GetCustomerTraitsRequest{CustomerId: uuid.NewString()})
			return e
		},
	} {
		if got := status.Code(call()); got != codes.PermissionDenied {
			t.Errorf("%s took a superuser's token (%s), want PermissionDenied", name, got)
		}
	}
}

// Core administers the suite's settings, the admin gate having run on its own side, so these take its
// system token - and an admin token does not reach them, which would be a second unguarded door
func TestASystemTokenAdministersTheSuitesSettings(t *testing.T) {
	c := signalsv1.NewSettingsServiceClient(testConn(t))
	sys := systemCtx(t)
	scope := &signalsv1.SettingScope{Kind: "default"}

	for name, call := range map[string]func() error{
		"ListDefinitions": func() error { _, e := c.ListDefinitions(sys, &signalsv1.ListDefinitionsRequest{}); return e },
		"GetCandidates": func() error {
			_, e := c.GetCandidates(sys, &signalsv1.GetCandidatesRequest{Path: "signals/lens/enabled", Scope: scope})
			return e
		},
		"ListInScopes": func() error { _, e := c.ListInScopes(sys, &signalsv1.ListInScopesRequest{Scope: scope}); return e },
		"ListAll":      func() error { _, e := c.ListAll(sys, &signalsv1.ListAllRequest{}); return e },
	} {
		if got := status.Code(call()); got == codes.PermissionDenied || got == codes.Unauthenticated {
			t.Errorf("%s refused core's own system token with %s", name, got)
		}
	}
	if got := status.Code(func() error {
		_, e := c.ListDefinitions(tokenCtx(t), &signalsv1.ListDefinitionsRequest{})
		return e
	}()); got != codes.PermissionDenied {
		t.Errorf("ListDefinitions took a superuser's token (%s), want PermissionDenied", got)
	}
}

// Core's settings page renders these once the suite's packages leave the module, so the declared set
// has to be exactly the Signals-store registry - a path short on either side is one an operator loses
func TestTheSuiteDeclaresExactlyItsOwnSettings(t *testing.T) {
	res, err := signalsv1.NewSettingsServiceClient(testConn(t)).
		ListDefinitions(systemCtx(t), &signalsv1.ListDefinitionsRequest{})
	if err != nil {
		t.Fatalf("ListDefinitions: %v", err)
	}
	declared := map[string]*signalsv1.SettingDefinition{}
	for _, d := range res.GetDefinitions() {
		declared[d.GetPath()] = d
	}
	for _, def := range config.AllDefinitions() {
		if def.Store != config.StoreSignals {
			if _, over := declared[def.Path]; over {
				t.Errorf("%s is core's, but the suite declared it", def.Path)
			}
			continue
		}
		d, ok := declared[def.Path]
		if !ok {
			t.Errorf("%s is a Signals setting the suite did not declare, so core's page would lose it", def.Path)
			continue
		}
		if d.GetLabel() != def.Label || d.GetDefault() != def.Default || d.GetType() != string(def.Type) {
			t.Errorf("%s declared as %q/%s/%q, want %q/%s/%q", def.Path,
				d.GetLabel(), d.GetType(), d.GetDefault(), def.Label, def.Type, def.Default)
		}
		if len(d.GetOptions()) != len(def.Options) {
			t.Errorf("%s declared %d options, want %d - a select with none is an empty picker", def.Path, len(d.GetOptions()), len(def.Options))
		}
	}
	if len(declared) == 0 {
		t.Fatal("the suite declared no settings at all")
	}
}

type fakeRevocations struct{ killed map[uuid.UUID]bool }

func (f fakeRevocations) Revoke(_ context.Context, id uuid.UUID)       { f.killed[id] = true }
func (f fakeRevocations) Revoked(_ context.Context, id uuid.UUID) bool { return f.killed[id] }

// A token minted before a logout still verifies - the list core publishes is what refuses it here
func TestARevokedSessionIsRefusedOnTheContract(t *testing.T) {
	list := fakeRevocations{killed: map[uuid.UUID]bool{}}
	c := signalsv1.NewPrismServiceClient(testConnWith(t, Deps{ServiceSecret: testSecret, Revocations: list, Channels: analytics.NewChannelRoster(nil)}))
	session := uuid.New()
	tok, err := auth.GenerateServiceToken(auth.ServiceClaims{AdminID: uuid.New(), SessionID: session, Email: "a@b.c", IsSuperuser: true}, signalsapi.Audience, testSecret, time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+tok)

	_, err = c.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: uuid.NewString()})
	if status.Code(err) == codes.Unauthenticated {
		t.Fatalf("a live session was refused: %v", err)
	}
	list.Revoke(ctx, session)
	_, err = c.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: uuid.NewString()})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("after revocation the call answered %v, want Unauthenticated", err)
	}
}

func TestAPartlyTakenMomentAnswersItsCount(t *testing.T) {
	res, err := enrollAnswer("order_shipped", 1, errors.New("flow b: db down"))
	if err != nil || res.GetEnrolled() != 1 || !res.GetIncomplete() {
		t.Fatalf("partial enroll = %v, %v, want 1 taken and marked incomplete - core would mail again what flow a sent", res, err)
	}
	if _, err := enrollAnswer("order_shipped", 0, errors.New("db down")); status.Code(err) != codes.Internal {
		t.Fatalf("nothing taken answered %v, want Internal so core sends its own mail", err)
	}
	if res, err := enrollAnswer("order_shipped", 2, nil); err != nil || res.GetEnrolled() != 2 || res.GetIncomplete() {
		t.Fatalf("clean enroll = %v, %v", res, err)
	}
}

func TestAnEmptyChannelPushIsRefused(t *testing.T) {
	c := signalsv1.NewIngestServiceClient(testConn(t))
	if got := status.Code(mustErr(c.SyncChannels(systemCtx(t), &signalsv1.SyncChannelsRequest{}))); got != codes.InvalidArgument {
		t.Fatalf("empty push answered %s, want InvalidArgument - it would prune every channel and its Pulse content", got)
	}
}

func TestEmailGroupsAreChangedOnlyWithAGlobalGrant(t *testing.T) {
	c := signalsv1.NewEmailBuilderServiceClient(testConnWith(t, Deps{ServiceSecret: testSecret, Channels: analytics.NewChannelRoster(nil), Builder: emailtemplate.NewService(nil, nil)}))
	ctx := tokenCtx(t, auth.Grant{Slug: "emails.manage", ChannelID: new(uuid.New())})
	id := uuid.New().String()
	for name, call := range map[string]func() error{
		"create": func() error { _, e := c.CreateEmailGroup(ctx, &signalsv1.CreateEmailGroupRequest{Name: "x"}); return e },
		"rename": func() error {
			_, e := c.RenameEmailGroup(ctx, &signalsv1.RenameEmailGroupRequest{Id: id, Name: "x"})
			return e
		},
		"delete": func() error { _, e := c.DeleteEmailGroup(ctx, &signalsv1.DeleteEmailGroupRequest{Id: id}); return e },
	} {
		if got := status.Code(call()); got != codes.PermissionDenied {
			t.Errorf("%s with one channel's grant answered %s, want PermissionDenied - groups hold every channel's emails", name, got)
		}
	}
}
