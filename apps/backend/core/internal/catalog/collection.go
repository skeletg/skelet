package catalog

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Collection struct {
	ID              uuid.UUID
	Slug            string
	IsPublished     bool
	SEONoindex      bool
	Content         *CollectionContent  // resolved for requested locale - nil if absent
	AllContent      []CollectionContent // all available translations (admin)
	CoverImage      *CollectionCover    // nil when no cover has been uploaded
	Metadata        Metadata
	PrivateMetadata Metadata
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CollectionContent struct {
	Locale          string
	Name            string
	Description     json.RawMessage // Lexical rich-text JSON - nil = no description
	MetaTitle       *string
	MetaDescription *string
	MetaKeywords    *string
	UpdatedAt       time.Time
}

type CollectionCover struct { // inline cover image - URL is computed at read time via objstore.Storage.PublicURL, it is not a DB column
	ID       uuid.UUID
	S3Key    string // raw S3 object key - used to derive URL
	URL      string // derived via objstore.Storage.PublicURL(S3Key)
	AltText  string
	Width    int32
	Height   int32
	MimeType string
	Status   string // "pending" | "ready"
}

type CreateCollectionInput struct {
	Slug            string
	IsPublished     bool
	SEONoindex      bool
	Metadata        Metadata
	PrivateMetadata Metadata
	Locale          string // initial content follows - locale and name are required at creation time
	Name            string
	Description     json.RawMessage
	MetaTitle       string
	MetaDescription string
	MetaKeywords    string
}

// UpdateCollectionInput uses nil pointers to mean "no change" - Slug can be changed - a URL redirect is created automatically if it was previously set
type UpdateCollectionInput struct {
	ID              uuid.UUID
	Slug            *string
	IsPublished     *bool
	SEONoindex      *bool
	Metadata        *Metadata
	PrivateMetadata *Metadata
}

type CollectionContentInput struct {
	Locale          string
	Name            string
	Description     json.RawMessage
	MetaTitle       string
	MetaDescription string
	MetaKeywords    string
}

type RequestCollectionCoverInput struct {
	CollectionID uuid.UUID
	Filename     string
	MimeType     string
	SizeBytes    int64
	AltText      string
}

type RequestCollectionCoverResult struct {
	Collection Collection
	UploadURL  string
	ExpiresAt  time.Time
}

var (
	ErrCollectionNotFound            = errors.New("collection not found")
	ErrCollectionContentNotFound     = errors.New("collection content not found for this locale")
	ErrCollectionProductNotFound     = errors.New("product is not in this collection")
	ErrCollectionProductAlreadyAdded = errors.New("product is already in this collection")
	ErrCollectionCoverNotPending     = errors.New("collection cover is not pending")
	ErrCollectionNoCover             = errors.New("collection has no cover image")
)
