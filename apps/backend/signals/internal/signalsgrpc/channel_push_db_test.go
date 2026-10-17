package signalsgrpc

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

func TestAPushThatCannotSayWhichChannelsAreDeactivatedPrunesNothing(t *testing.T) {
	conn := testdb.RequireSignals(t)
	live, paused := uuid.New(), uuid.New()
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM channel_roster WHERE channel_id IN ($1, $2)`, live, paused)
		_, _ = conn.Exec(`DELETE FROM flows WHERE channel_id = $1`, paused)
	})
	if _, err := conn.Exec(`INSERT INTO flows (channel_id, name, trigger_type) VALUES ($1, 'Test', 'order_placed')`, paused); err != nil {
		t.Fatal(err)
	}
	c := signalsv1.NewIngestServiceClient(testConnWith(t, Deps{ServiceKeys: testKeys, Channels: analytics.NewChannelRoster(conn)}))
	yes := true
	if _, err := c.SyncChannels(systemCtx(t), &signalsv1.SyncChannelsRequest{Channels: []*signalsv1.ChannelEntry{
		{ChannelId: live.String(), IsActive: &yes}, {ChannelId: paused.String(), IsActive: &yes},
	}}); err != nil {
		t.Fatal(err)
	}

	res, err := c.SyncChannels(systemCtx(t), &signalsv1.SyncChannelsRequest{Channels: []*signalsv1.ChannelEntry{{ChannelId: live.String()}}})
	if err != nil {
		t.Fatal(err)
	}
	var onRoster, flows int
	_ = conn.QueryRow(`SELECT count(*) FROM channel_roster WHERE channel_id = $1`, paused).Scan(&onRoster)
	_ = conn.QueryRow(`SELECT count(*) FROM flows WHERE channel_id = $1`, paused).Scan(&flows)
	if res.GetPruned() != 0 || onRoster != 1 || flows != 1 {
		t.Fatalf("pruned %d, roster %d, flows %d - an old core leaves deactivated channels out, and they are not deleted", res.GetPruned(), onRoster, flows)
	}
}
