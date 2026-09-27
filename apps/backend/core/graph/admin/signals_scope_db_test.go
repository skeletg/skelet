package admin

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"github.com/skeletg/skelet/apps/backend/core/internal/audienceio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

type fakeDestinations struct {
	signalsv1.DestinationServiceClient
	channelID string
	synced    []string
}

func (f *fakeDestinations) GetDestination(_ context.Context, in *signalsv1.GetDestinationRequest, _ ...grpc.CallOption) (*signalsv1.GetDestinationResponse, error) {
	return &signalsv1.GetDestinationResponse{Destination: &signalsv1.EventDestination{Id: in.GetId(), ChannelId: f.channelID}}, nil
}

func (f *fakeDestinations) SyncAudience(_ context.Context, in *signalsv1.SyncAudienceRequest, _ ...grpc.CallOption) (*signalsv1.SyncAudienceResponse, error) {
	f.synced = append(f.synced, in.GetIdentifiers()...)
	return &signalsv1.SyncAudienceResponse{IdentifiersSent: int32(len(in.GetIdentifiers()))}, nil
}

type fakeSegmentMembers struct {
	signalsv1.SegmentServiceClient
	members []uuid.UUID
}

func (f *fakeSegmentMembers) ListMembers(context.Context, *signalsv1.ListMembersRequest, ...grpc.CallOption) (*signalsv1.ListMembersResponse, error) {
	out := &signalsv1.ListMembersResponse{}
	for _, id := range f.members {
		out.Members = append(out.Members, &signalsv1.SegmentMember{CustomerId: id.String()})
	}
	return out, nil
}

type fakeStarterSeed struct {
	signalsv1.FlowServiceClient
	seeded int
}

func (f *fakeStarterSeed) SeedStarterContent(context.Context, *signalsv1.SeedStarterContentRequest, ...grpc.CallOption) (*signalsv1.SeedStarterContentResponse, error) {
	f.seeded++
	return &signalsv1.SeedStarterContentResponse{}, nil
}

func fakeSignalsClient(t *testing.T) *signalsclient.Client {
	t.Helper()
	c, err := signalsclient.New("signals.invalid:1", "scope-test-secret", time.Minute, nil)
	if err != nil {
		t.Fatalf("signals client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func adminWithGrants(grants ...auth.Grant) context.Context {
	id := uuid.New()
	ctx := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: id, Email: "a@b.c", IsSuperuser: len(grants) == 0})
	return auth.WithGrantChecker(ctx, auth.NewGrantChecker(id, auth.NewGrantSet(grants)))
}

func TestSeedingStarterContentNeedsFlowsManageToo(t *testing.T) {
	channel := uuid.New()
	flows := &fakeStarterSeed{}
	client := fakeSignalsClient(t)
	client.Flows = flows
	r := &mutationResolver{&Resolver{SignalsEnabled: true, SignalsAPI: client}}
	on := func(slug string) auth.Grant { return auth.Grant{Slug: slug, ChannelID: &channel} }

	if _, err := r.SeedPulseStarterContent(adminWithCachedGrants(on("emails.manage")), channel.String()); err == nil || flows.seeded != 0 {
		t.Errorf("seeded with emails.manage alone: err=%v, calls=%d", err, flows.seeded)
	}
	if _, err := r.SeedPulseStarterContent(adminWithCachedGrants(on("emails.manage"), on("flows.manage")), channel.String()); err != nil || flows.seeded != 1 {
		t.Errorf("seeding with both grants: err=%v, calls=%d", err, flows.seeded)
	}
}

func seedChannelCustomer(t *testing.T, conn *sql.DB, channelID uuid.UUID) (uuid.UUID, string) {
	t.Helper()
	var groupID, id uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Skipf("no customer group to seed against: %v", err)
	}
	addr := "audience-" + uuid.NewString()[:8] + "@example.invalid"
	if err := conn.QueryRow(
		`INSERT INTO customers (email, password_hash, customer_group_id, channel_id) VALUES ($1, 'x', $2, $3) RETURNING id`,
		addr, groupID, channelID).Scan(&id); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, id) })
	return id, addr
}

func TestSyncAudienceSendsOnlyTheDestinationChannelsCustomers(t *testing.T) {
	conn := testdb.Require(t)
	var own uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&own); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}
	var other uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO channels (slug, name, currency_code, default_locale)
		 SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`,
		"audience-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatalf("seed second channel: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	ownID, ownAddr := seedChannelCustomer(t, conn, own)
	otherID, _ := seedChannelCustomer(t, conn, other)

	dests := &fakeDestinations{channelID: own.String()}
	client := fakeSignalsClient(t)
	client.Destination = dests
	client.Segments = &fakeSegmentMembers{members: []uuid.UUID{ownID, otherID}}
	r := &mutationResolver{&Resolver{SignalsEnabled: true, SignalsAPI: client, Queries: db.New(conn)}}

	n, err := r.SyncAudience(adminWithGrants(), uuid.NewString(), uuid.NewString())
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if want := []string{audienceio.HashIdentifier(ownAddr)}; n != 1 || !slices.Equal(dests.synced, want) {
		t.Errorf("synced %d identifiers %v, want only the destination channel's customer %v", n, dests.synced, want)
	}
}
