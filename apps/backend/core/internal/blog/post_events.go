package blog

import (
	"github.com/google/uuid"
	contentpkg "github.com/skeletg/skelet/apps/backend/core/internal/content"
)

const (
	EventPostPublishing     = "blog.post.publishing"
	EventPostPublished      = "blog.post.published"
	EventPostUnpublished    = "blog.post.unpublished"
	EventPostCreated        = "blog.post.created" // this CRUD/content/listing set keeps search indexes in sync with every shape change to the post, not just the publish transition - async-only
	EventPostUpdated        = "blog.post.updated"
	EventPostDeleted        = "blog.post.deleted"
	EventPostContentChanged = "blog.post.content.changed"
	EventPostListingChanged = "blog.post.listing.changed"
)

type PostPublishing struct { // fires BEFORE a post transitions to published, by admin action or the scheduler promoting it - sync handlers may abort (legal review pending, a lockdown window) or no-op with a {"reject": bool, "reason": string} response, and a scheduled post they refuse stays scheduled
	PostID   uuid.UUID
	Slug     string
	AuthorID *uuid.UUID
}

func (*PostPublishing) Type() string { return EventPostPublishing }

func (e *PostPublishing) ApplyWebhookResponse(body []byte) (bool, string, error) {
	return contentpkg.DecodePublishVeto(body)
}

type PostPublished struct { // fires AFTER a post moves to status=published, whether by admin action, the scheduler promoting a scheduled post, or any other path - async-only
	PostID   uuid.UUID
	Slug     string
	AuthorID *uuid.UUID
}

func (*PostPublished) Type() string { return EventPostPublished }

type PostUnpublished struct {
	PostID    uuid.UUID
	Slug      string
	NewStatus PostStatus
}

func (*PostUnpublished) Type() string { return EventPostUnpublished }

type PostCreated struct { // fires AFTER a new blog_posts row commits, async-only - the search indexer is the primary subscriber
	PostID uuid.UUID
	Slug   string
}

func (*PostCreated) Type() string { return EventPostCreated }

type PostUpdated struct {
	PostID uuid.UUID
	Slug   string
}

func (*PostUpdated) Type() string { return EventPostUpdated }

type PostDeleted struct { // fires AFTER a post row delete commits, async-only - subscribers must use the snapshot fields, since the row is gone
	PostID uuid.UUID
	Slug   string
}

func (*PostDeleted) Type() string { return EventPostDeleted }

type PostContentChanged struct {
	PostID uuid.UUID
	Locale string
}

func (*PostContentChanged) Type() string { return EventPostContentChanged }

type PostListingChanged struct {
	PostID      uuid.UUID
	ChannelID   uuid.UUID
	IsPublished bool
	Cleared     bool
}

func (*PostListingChanged) Type() string { return EventPostListingChanged }
