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

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// The roster push is the one destructive write on this surface: its sweep deletes every customer the stream did not carry, and their traits and memberships with them. So the handler is exercised against a real database, not a nil service

func rosterServer(t *testing.T) (signalsv1.IngestServiceClient, *signalsdb.Queries, uuid.UUID) {
	t.Helper()
	conn := testdb.RequireSignals(t)
	channelID := uuid.New()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customer_roster WHERE channel_id = $1`, channelID) })

	qs := signalsdb.New(conn)
	lis := bufconn.Listen(1 << 20)
	srv := NewServer(Deps{ServiceSecret: testSecret, Analytics: analytics.NewRecorder(qs, nil)})
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

// A push that fails partway must leave the roster exactly as it was. A client that simply vanishes is covered by its cancelled context failing the sweep too, so this drives the case where the stream breaks on its own terms and the context is still live - the one only the handler's ordering catches
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

// Every field on the roster crosses the wire through two hand-written mappings, and one dropped in either reads downstream as "core never had one"
// An SMS node would skip a subscriber who is opted in, and a campaign would address nobody
func TestAPushedEntryArrivesWithEveryFieldItLeftWith(t *testing.T) {
	client, _, channelID := rosterServer(t)
	conn := testdb.RequireSignals(t)
	customerID := uuid.New()
	groupID := uuid.New()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customer_roster WHERE customer_id = $1`, customerID) })

	sent := &signalsv1.RosterEntry{
		CustomerId: customerID.String(), ChannelId: channelID.String(),
		Email: "ada@test", FirstName: "Ada", Phone: "+15550001111",
		IsActive: true, CustomerGroupId: groupID.String(),
		MarketingOptedIn: true, SmsOptedIn: true,
	}
	if _, err := client.UpsertRosterEntry(rosterCtx(t), &signalsv1.UpsertRosterEntryRequest{Customer: sent}); err != nil {
		t.Fatalf("push the entry: %v", err)
	}

	var email, firstName, phone string
	var active, marketing, sms bool
	var group uuid.NullUUID
	if err := conn.QueryRow(
		`SELECT email, first_name, phone, is_active, customer_group_id, marketing_opted_in, sms_opted_in
		 FROM customer_roster WHERE customer_id = $1`, customerID,
	).Scan(&email, &firstName, &phone, &active, &group, &marketing, &sms); err != nil {
		t.Fatalf("read the entry back: %v", err)
	}

	for _, f := range []struct{ name, got, want string }{
		{"email", email, "ada@test"},
		{"first name", firstName, "Ada"},
		{"phone", phone, "+15550001111"},
	} {
		if f.got != f.want {
			t.Errorf("%s = %q, want %q", f.name, f.got, f.want)
		}
	}
	if !active || !marketing || !sms {
		t.Errorf("flags = active:%t marketing:%t sms:%t, want all true", active, marketing, sms)
	}
	if !group.Valid || group.UUID != groupID {
		t.Errorf("group = %v, want %s - a group-audience campaign filters on it", group, groupID)
	}
}
