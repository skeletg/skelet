package urlrewrite

import "github.com/google/uuid"

const (
	EventChanged = "urlrewrite.changed"
)

// Fires AFTER any url_rewrite row is created, updated or deleted - async-only, with the CDN cache purge plugin the primary subscriber, since a changed canonical or redirect needs the prior path evicted from edge caches
type Changed struct {
	ID         uuid.UUID
	Path       string
	EntityType string
	EntityID   uuid.UUID
	ChannelID  *uuid.UUID
	Deleted    bool
}

func (*Changed) Type() string { return EventChanged }
