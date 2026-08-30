package channel

import "github.com/google/uuid"

const (
	EventLocalesChanged = "channel.locales.changed"
	EventDeleted        = "channel.deleted"
)

// Fires after a channel's locale set changes - the search indexer rebuilds per-locale docs, since removed locales leave stale ones and added locales have none
type LocalesChanged struct {
	ChannelID uuid.UUID
}

func (*LocalesChanged) Type() string { return EventLocalesChanged }

type Deleted struct {
	ChannelID uuid.UUID
}

func (*Deleted) Type() string { return EventDeleted }
