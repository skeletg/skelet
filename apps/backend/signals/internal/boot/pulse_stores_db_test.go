package boot

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

func TestADeactivatedChannelStillResolvesForTheEnginesToDecide(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	roster := analytics.NewChannelRoster(conn)
	id := uuid.New()
	if _, _, err := roster.SyncChannels(ctx, []analytics.ChannelEntry{{ID: id, Name: "Shop", IsActive: false}}); err != nil {
		t.Fatal(err)
	}
	stores := newRosterStores(roster)
	if f, err := (flowStores{stores}).Get(ctx, id); err != nil || f.Active {
		t.Errorf("flow store = %+v, %v, want the channel back marked inactive so transactional mail still goes", f, err)
	}
	if c, err := (campaignStores{stores}).Get(ctx, id); err != nil || c.Active {
		t.Errorf("campaign store = %+v, %v, want the channel back marked inactive so previews still render", c, err)
	}
}
