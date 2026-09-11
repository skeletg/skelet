package catalog

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// An image attached to a product - the binary lives in S3 at S3Key, URL is computed at read time via media.Storage.PublicURL rather than stored
type ProductMedia struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	S3Key     string
	URL       string
	MimeType  string
	SizeBytes int64
	Width     int32
	Height    int32
	AltText   string
	SortOrder int32
	Status    MediaStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Two-phase upload - "pending" means a presigned URL was handed out but the client isn't confirmed done, "ready" means the object exists and can surface in the UI
type MediaStatus string

const (
	MediaStatusPending MediaStatus = "pending"
	MediaStatusReady   MediaStatus = "ready"
)

// A named slot pointing at one ready media item per product - a product may set any subset, unset roles return null
type MediaRole string

const (
	MediaRoleMain      MediaRole = "main"      // primary image on PDP
	MediaRoleThumbnail MediaRole = "thumbnail" // product cards, cart, search
	MediaRoleOGImage   MediaRole = "og_image"  // social sharing meta tag
)

var AllMediaRoles = []MediaRole{MediaRoleMain, MediaRoleThumbnail, MediaRoleOGImage}

// Picks the storage subdirectory so content-embedded images stay separate from gallery and role-oriented imagery
type MediaUploadTarget string

const (
	MediaUploadTargetGallery          MediaUploadTarget = "gallery"
	MediaUploadTargetDescription      MediaUploadTarget = "description"
	MediaUploadTargetShortDescription MediaUploadTarget = "short-description"
)

type RequestMediaUploadInput struct {
	ProductID uuid.UUID
	Filename  string // informational - extension used for the S3 key
	MimeType  string
	SizeBytes int64
	AltText   string
	SortOrder int32
	// Optional - description-focused targets take precedence over any role-derived path
	Target *MediaUploadTarget
	// Optional - picks the S3 path segment but does NOT assign the role, that needs a separate setProductMediaRole call
	Role *MediaRole
}

type RequestMediaUploadResult struct {
	Media     ProductMedia
	UploadURL string
	ExpiresAt time.Time
}

type ConfirmMediaUploadInput struct {
	MediaID uuid.UUID
	Width   int32
	Height  int32
}

type UpdateProductMediaInput struct {
	ID        uuid.UUID
	AltText   *string
	SortOrder *int32
}

// Minimal cleanup-query result - enough to also delete the orphaned S3 object
type PendingMediaRow struct {
	ID    uuid.UUID
	S3Key string
}

var (
	ErrMediaNotFound     = errors.New("media not found")
	ErrMediaWrongProduct = errors.New("media does not belong to this product")
	ErrMediaNotPending   = errors.New("media is not pending")
	ErrInvalidMimeType   = errors.New("mime type not allowed")
	ErrFileTooLarge      = errors.New("file exceeds max upload size")
	ErrUploadNotReceived = errors.New("upload not received at s3")
)
