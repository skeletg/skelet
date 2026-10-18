package webhook

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
)

func TestChannelEventsReachTheirChannelsWebhooks(t *testing.T) {
	ch := uuid.New()
	for name, e := range map[string]events.Event{
		"listing changed": &catalog.ProductListingChanged{ChannelID: ch},
		"rewrite changed": &urlrewrite.Changed{ChannelID: &ch},
		"locales changed": &channel.LocalesChanged{ChannelID: ch},
		"channel created": &channel.Created{ChannelID: ch},
		"channel deleted": &channel.Deleted{ChannelID: ch},
	} {
		if got := eventStoreID(e); got == nil || *got != ch {
			t.Errorf("%s scoped to %v, want its channel - a webhook on that channel would never receive it", name, got)
		}
	}
	if got := eventStoreID(&urlrewrite.Changed{}); got != nil {
		t.Errorf("a global rewrite scoped to %v, want none", got)
	}
}
