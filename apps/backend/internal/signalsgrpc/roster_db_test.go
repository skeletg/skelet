package signalsgrpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// The roster push is the one destructive write on this surface: its sweep deletes every customer the
// stream did not carry, and their traits and memberships with them. So the handler is exercised
// against a real database, not a nil service

func rosterServer(t *testing.T) (signalsv1.IngestServiceClient, *signalsdb.Queries, uuid.UUID) {
	t.Helper()
	conn := testdb.RequireSignals(t)
	channelID := uuid.New()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customer_roster WHERE channel_id = $1`, channelID) })

	qs := signalsdb.New(conn)
	lis := bufconn.Listen(1 << 20)
	srv := NewServer(Deps{JWTSecret: testSecret, Analytics: analytics.NewRecorder(nil, qs)})
	go func() { _ = srv.Serve(lis) }()
	cc, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close(); srv.Stop() })
	return signalsv1.NewIngestServiceClient(cc), qs, channelID
}

func rosterCtx(t *testing.T) context.Context {
	t.Helper()
	tok, err := auth.GenerateServiceToken(auth.ServiceClaims{System: true}, signalsapi.Audience, testSecret, time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+tok)
}

func rosterCount(t *testing.T, qs *signalsdb.Queries, channelID uuid.UUID) int {
	t.Helper()
	ids, err := qs.ChannelRosterCustomerIDs(context.Background(), channelID)
	if err != nil {
		t.Fatalf("count roster: %v", err)
	}
	return len(ids)
}

func push(t *testing.T, c signalsv1.IngestServiceClient, ctx context.Context, channelID uuid.UUID, customers ...uuid.UUID) {
	t.Helper()
	stream, err := c.SyncCustomerRoster(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	entries := make([]*signalsv1.RosterEntry, 0, len(customers))
	for _, id := range customers {
		entries = append(entries, &signalsv1.RosterEntry{CustomerId: id.String(), ChannelId: channelID.String()})
	}
	if len(entries) > 0 {
		if err := stream.Send(&signalsv1.SyncCustomerRosterRequest{Customers: entries}); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

// A push that fails partway must leave the roster exactly as it was. A client that simply vanishes is
// covered by its cancelled context failing the sweep too, so this drives the case where the stream
// breaks on its own terms and the context is still live - the one only the handler's ordering catches
func TestARejectedRosterPagePrunesNothing(t *testing.T) {
	c, qs, channelID := rosterServer(t)
	ctx := rosterCtx(t)
	a, b := uuid.New(), uuid.New()
	push(t, c, ctx, channelID, a, b)
	if got := rosterCount(t, qs, channelID); got != 2 {
		t.Fatalf("first push landed %d customers, want 2", got)
	}

	stream, err := c.SyncCustomerRoster(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	// A page the server refuses - it must end the push before the sweep, not after
	if err := stream.Send(&signalsv1.SyncCustomerRosterRequest{
		Customers: []*signalsv1.RosterEntry{{CustomerId: "not-a-uuid", ChannelId: channelID.String()}},
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := stream.CloseAndRecv(); err == nil {
		t.Fatal("the server accepted an unparseable customer id")
	}

	if got := rosterCount(t, qs, channelID); got != 2 {
		t.Errorf("a refused page left %d customers, want 2 - the sweep ran on a push that never completed", got)
	}
}

// And the completed case still sweeps, or a customer deleted in core keeps their traits forever
func TestACompletedRosterStreamSweepsWhatItDidNotCarry(t *testing.T) {
	c, qs, channelID := rosterServer(t)
	ctx := rosterCtx(t)
	stays, goes := uuid.New(), uuid.New()
	push(t, c, ctx, channelID, stays, goes)
	if got := rosterCount(t, qs, channelID); got != 2 {
		t.Fatalf("first push landed %d, want 2", got)
	}

	push(t, c, ctx, channelID, stays) // the second push no longer carries `goes`

	ids, err := qs.ChannelRosterCustomerIDs(context.Background(), channelID)
	if err != nil {
		t.Fatalf("read roster: %v", err)
	}
	if len(ids) != 1 || ids[0] != stays {
		t.Errorf("roster holds %v, want only %s - the sweep did not drop the customer the push omitted", ids, stays)
	}
}
