package segment

import "github.com/google/uuid"

const EventRefreshed = "segment.refreshed" // one event per refresh, not per member, so bulk sweeps stay webhook-safe

type Refreshed struct {
	SegmentID uuid.UUID
	Slug      string
	Matched   int
}

func (*Refreshed) Type() string { return EventRefreshed }
