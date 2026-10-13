package cms

import (
	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/syncveto"
)

// Event types the CMS domain emits - subscribers commonly trigger a static-site rebuild or CDN purge when a page goes live
const (
	EventPagePublishing  = "cms.page.publishing"
	EventPagePublished   = "cms.page.published"
	EventPageUnpublished = "cms.page.unpublished"
	EventPageChanged     = "cms.page.changed"
)

// Fires BEFORE a page transitions to is_published - sync handlers may abort for a pending review or a content lockdown - Response {"reject": bool, "reason": string}
type PagePublishing struct {
	PageID     uuid.UUID
	PageTypeID uuid.UUID
	Slug       string
	URLPath    string
}

func (*PagePublishing) Type() string { return EventPagePublishing }

func (e *PagePublishing) ApplyWebhookResponse(body []byte) (bool, string, error) {
	return syncveto.Decode(body)
}

type PagePublished struct {
	PageID     uuid.UUID
	PageTypeID uuid.UUID
	Slug       string
	URLPath    string
}

func (*PagePublished) Type() string { return EventPagePublished }

type PageUnpublished struct {
	PageID     uuid.UUID
	PageTypeID uuid.UUID
	Slug       string
	URLPath    string
}

func (*PageUnpublished) Type() string { return EventPageUnpublished }

// A live page's content, listings, tags or URL changed, or it was deleted - Paths are URLs it answered at before the change, which a purge must reach beside its current ones
type PageChanged struct {
	PageID uuid.UUID
	Paths  []string
}

func (*PageChanged) Type() string { return EventPageChanged }
