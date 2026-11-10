package admin

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

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
	c, err := signalsclient.New("signals.invalid:1", signalsclient.Transport{}, auth.MustServiceKeys("scope-test-secret"), time.Minute, nil)
	if err != nil {
		t.Fatalf("signals client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
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
