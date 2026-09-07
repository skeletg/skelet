package segment

import (
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/webhook"
)

// Segments declare their own webhook event, so core's registry doesn't have to name a Synapse one
func init() {
	webhook.RegisterEvent(webhook.EventDefinition{
		Type:        EventRefreshed,
		Kind:        webhook.KindAsync,
		Name:        "Segment Refreshed",
		Description: "Fires after a dynamic (rule-based) segment recomputes its membership. Payload: SegmentID, Slug, Matched (member count). One event per refresh - bulk sweeps stay webhook-safe.",
	})
}

const EventRefreshed = "segment.refreshed" // one event per refresh, not per member, so bulk sweeps stay webhook-safe

type Refreshed struct {
	SegmentID uuid.UUID
	Slug      string
	Matched   int
}

func (*Refreshed) Type() string { return EventRefreshed }
