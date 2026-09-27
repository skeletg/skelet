package signalsgrpc

import (
	"context"
	"database/sql"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

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
	srv := NewServer(Deps{ServiceSecret: testSecret, Analytics: analytics.NewRecorder(qs, nil), CustomerRoster: analytics.NewRoster(qs)})
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

func TestRosterAsksForAFullSyncUntilOneLands(t *testing.T) {
	ingest, _, channelID := rosterServer(t)
	ctx := rosterCtx(t)
	if _, err := testdb.RequireSignals(t).Exec(`DELETE FROM customer_roster_state`); err != nil {
		t.Fatalf("reset roster state: %v", err)
	}

	up, err := ingest.UpsertRosterEntry(ctx, &signalsv1.UpsertRosterEntryRequest{
		Customer: &signalsv1.RosterEntry{CustomerId: uuid.NewString(), ChannelId: channelID.String()},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if up.GetRosterVersion() != analytics.RosterVersion || !up.GetNeedsFullSync() {
		t.Errorf("before any full sync: version=%d needs=%v, want %d and true", up.GetRosterVersion(), up.GetNeedsFullSync(), analytics.RosterVersion)
	}
	st, err := ingest.RosterStatus(ctx, &signalsv1.RosterStatusRequest{})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.GetNeedsFullSync() || st.GetFilledVersion() != 0 || st.GetFilledAt() != nil {
		t.Errorf("status before a sync = %+v, want behind with nothing filled", st)
	}

	stream, err := ingest.SyncCustomerRoster(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if err := stream.Send(&signalsv1.SyncCustomerRosterRequest{Customers: []*signalsv1.RosterEntry{
		{CustomerId: uuid.NewString(), ChannelId: channelID.String()},
	}}); err != nil {
		t.Fatalf("send: %v", err)
	}
	res, err := stream.CloseAndRecv()
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if res.GetRosterVersion() != analytics.RosterVersion {
		t.Errorf("sync answered version %d, want %d", res.GetRosterVersion(), analytics.RosterVersion)
	}
	st, err = ingest.RosterStatus(ctx, &signalsv1.RosterStatusRequest{})
	if err != nil {
		t.Fatalf("status after sync: %v", err)
	}
	if st.GetNeedsFullSync() || st.GetFilledVersion() != analytics.RosterVersion || st.GetFilledAt() == nil {
		t.Errorf("status after a full sync = %+v, want filled at the current version", st)
	}
}

// Core's lifetime order totals land on the roster row - the RFM scores read them from there
func TestAPushedEntryCarriesItsLifetimeOrders(t *testing.T) {
	client, _, channelID := rosterServer(t)
	conn := testdb.RequireSignals(t)
	buyer, never := uuid.New(), uuid.New()
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM customer_roster WHERE customer_id = ANY($1)`, pq.Array([]uuid.UUID{buyer, never}))
	})
	first, last := time.Date(2023, 3, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
	for _, e := range []*signalsv1.RosterEntry{
		{CustomerId: buyer.String(), ChannelId: channelID.String(), OrderCount: 10, MonetaryCents: 123400,
			FirstOrderAt: timestamppb.New(first), LastOrderAt: timestamppb.New(last)},
		{CustomerId: never.String(), ChannelId: channelID.String()},
	} {
		if _, err := client.UpsertRosterEntry(rosterCtx(t), &signalsv1.UpsertRosterEntryRequest{Customer: e}); err != nil {
			t.Fatalf("push: %v", err)
		}
	}
	read := func(id uuid.UUID) (int32, int64, sql.NullTime, sql.NullTime) {
		var n int32
		var cents int64
		var f, l sql.NullTime
		if err := conn.QueryRow(`SELECT order_count, monetary_cents, first_order_at, last_order_at FROM customer_roster WHERE customer_id = $1`, id).Scan(&n, &cents, &f, &l); err != nil {
			t.Fatal(err)
		}
		return n, cents, f, l
	}
	if n, cents, f, l := read(buyer); n != 10 || cents != 123400 || !f.Time.Equal(first) || !l.Time.Equal(last) {
		t.Errorf("buyer: %d orders, %d cents, first %v, last %v", n, cents, f, l)
	}
	if n, cents, f, l := read(never); n != 0 || cents != 0 || f.Valid || l.Valid {
		t.Errorf("never ordered: %d orders, %d cents, first %v, last %v - want zeros and NULLs", n, cents, f, l)
	}
}

// The full sync is what backfills the totals after an upgrade, so a streamed page must carry them too
func TestAStreamedPageCarriesLifetimeOrders(t *testing.T) {
	c, _, channelID := rosterServer(t)
	conn := testdb.RequireSignals(t)
	buyer := uuid.New()
	last := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
	stream, err := c.SyncCustomerRoster(rosterCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&signalsv1.SyncCustomerRosterRequest{Customers: []*signalsv1.RosterEntry{{
		CustomerId: buyer.String(), ChannelId: channelID.String(), OrderCount: 3, MonetaryCents: 9900,
		FirstOrderAt: timestamppb.New(last.AddDate(-2, 0, 0)), LastOrderAt: timestamppb.New(last),
	}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatal(err)
	}
	var n int32
	var cents int64
	var l time.Time
	if err := conn.QueryRow(`SELECT order_count, monetary_cents, last_order_at FROM customer_roster WHERE customer_id = $1`, buyer).Scan(&n, &cents, &l); err != nil {
		t.Fatal(err)
	}
	if n != 3 || cents != 9900 || !l.Equal(last) {
		t.Errorf("streamed buyer: %d orders, %d cents, last %v", n, cents, l)
	}
}

// Only the two writes an erased customer could still arrive on - any other spine call hits the nil interface and fails the test
type countingSpine struct {
	analytics.SpineStore
	appends, engagementReads int
}

func (c *countingSpine) AppendEvent(context.Context, analytics.SpineEvent) (bool, error) {
	c.appends++
	return true, nil
}

func (c *countingSpine) RecentEngagementEvents(context.Context, uuid.UUID, uuid.UUID, time.Time) ([]analytics.EngagementCandidate, error) {
	c.engagementReads++
	return nil, nil
}

func TestAnErasedCustomerNeverReachesTheSpineAgain(t *testing.T) {
	conn := testdb.RequireSignals(t)
	qs := signalsdb.New(conn)
	spine := &countingSpine{}
	cc := dial(t, Deps{ServiceSecret: testSecret, Analytics: analytics.NewRecorder(qs, spine), CustomerRoster: analytics.NewRoster(qs)})
	c := signalsv1.NewIngestServiceClient(cc)
	ctx := rosterCtx(t)
	id, channelID := uuid.New(), uuid.New()
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM erased_customers WHERE customer_id = $1`, id)
		_, _ = conn.Exec(`DELETE FROM gdpr_identity_erasures WHERE customer_id = $1`, id)
	})
	cust := id.String()
	activity := func() {
		t.Helper()
		if _, err := c.RecordActivity(ctx, &signalsv1.RecordActivityRequest{EventName: "order_shipped", Source: "commerce", ChannelId: channelID.String(), CustomerId: &cust, DedupeKey: "act:" + uuid.NewString()}); err != nil {
			t.Fatalf("record activity: %v", err)
		}
	}
	conversion := func() {
		t.Helper()
		if _, err := c.RecordConversion(ctx, &signalsv1.RecordConversionRequest{CustomerId: cust, ChannelId: channelID.String(), OrderId: uuid.NewString(), RevenueCents: 100, Currency: "USD"}); err != nil {
			t.Fatalf("record conversion: %v", err)
		}
	}

	activity()
	conversion()
	if spine.appends != 1 || spine.engagementReads != 1 {
		t.Fatalf("before the erasure: %d appends, %d attribution reads - want 1 and 1", spine.appends, spine.engagementReads)
	}

	if _, err := c.EraseCustomerIdentities(ctx, &signalsv1.EraseCustomerIdentitiesRequest{CustomerId: cust, ErasedAt: timestamppb.Now()}); err != nil {
		t.Fatalf("erase: %v", err)
	}
	activity()
	conversion()
	if spine.appends != 1 {
		t.Error("an order event that arrived after the erasure put the customer's id back on the spine")
	}
	if spine.engagementReads != 1 {
		t.Error("a conversion for an erased customer went on to attribution")
	}
}
