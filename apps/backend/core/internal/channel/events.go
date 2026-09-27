package channel

import "github.com/google/uuid"

const (
	EventLocalesChanged = "channel.locales.changed"
	EventCreated        = "channel.created"
	EventDeleted        = "channel.deleted"
)

// Fires after a channel's locale set changes - the search indexer rebuilds per-locale docs, since removed locales leave stale ones and added locales have none
type LocalesChanged struct {
	ChannelID uuid.UUID
}

func (*LocalesChanged) Type() string { return EventLocalesChanged }

func (e *LocalesChanged) EventChannelID() uuid.UUID { return e.ChannelID }

// Fires after a channel is created - the transactional emails and their delivery flows are seeded from here, so a new channel sends receipts and account mail without anyone running the CLI
type Created struct {
	ChannelID uuid.UUID
}

func (*Created) Type() string { return EventCreated }

func (e *Created) EventChannelID() uuid.UUID { return e.ChannelID }

type Deleted struct {
	ChannelID uuid.UUID
}

func (*Deleted) Type() string { return EventDeleted }

func (e *Deleted) EventChannelID() uuid.UUID { return e.ChannelID }
