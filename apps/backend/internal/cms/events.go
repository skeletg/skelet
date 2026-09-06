package cms

import (
	"github.com/google/uuid"
	contentpkg "github.com/skeletg/skelet/apps/backend/internal/content"
)

// Event types the CMS domain emits - subscribers commonly trigger a static-site rebuild or CDN purge when a page goes live
const (
	EventPagePublishing  = "cms.page.publishing"
	EventPagePublished   = "cms.page.published"
	EventPageUnpublished = "cms.page.unpublished"
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
	return contentpkg.DecodePublishVeto(body)
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
