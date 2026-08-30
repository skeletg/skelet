package catalog

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// One facet's per-category presentation - SortOrder pins it ahead of the attribute
// default on that category's PLP, Hidden drops it from the sidebar entirely
type FacetOverride struct {
	AttributeID   uuid.UUID
	AttributeSlug string
	SortOrder     int32
	Hidden        bool
}

type Category struct {
	ID              uuid.UUID
	ParentID        *uuid.UUID
	Slug            string
	URLPath         string // full hierarchical path: shoes/mens/running
	Position        int32
	IsPublished     bool
	SEONoindex      bool
	Content         *CategoryContent // populated only when a locale is requested
	AllContent      []CategoryContent
	CoverImage      *CategoryCover
	Ancestors       []Category               // root→immediate parent - populated on demand
	Children        []Category               // immediate children - populated on demand
	Products        []Product                // products directly assigned - populated on demand
	ChannelListings []CategoryChannelListing // per-store visibility rows, populated by the admin detail loader - storefront resolvers use listing-aware queries instead of reading this slice
	Metadata        Metadata
	PrivateMetadata Metadata
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CategoryChannelListing controls whether a category appears on a particular storefront - Without a row, the category is treated as not-listed (hidden) on that store - SortOrderOverride lets a store reorder a node within its parent without touching the global Position
type CategoryChannelListing struct {
	ChannelID         uuid.UUID
	IsPublished       bool
	SortOrderOverride *int32
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// SetCategoryChannelListingInput is the payload for the admin upsert - A nil SortOrderOverride keeps the override unset (i.e. inherit category.Position)
type SetCategoryChannelListingInput struct {
	CategoryID        uuid.UUID
	ChannelID         uuid.UUID
	IsPublished       bool
	SortOrderOverride *int32
}

type CategoryContent struct {
	Locale           string
	Name             string
	ShortDescription json.RawMessage
	Description      json.RawMessage
	MetaTitle        *string
	MetaDescription  *string
	MetaKeywords     *string
	UpdatedAt        time.Time
}

type CategoryCover struct { // inline cover image, same pattern as CollectionCover
	ID       uuid.UUID
	S3Key    string
	URL      string // derived via media.Storage.PublicURL(S3Key)
	AltText  string
	Width    int32
	Height   int32
	MimeType string
	Status   string // "pending" | "ready"
}

type CreateCategoryInput struct {
	ParentID         *uuid.UUID
	Slug             string
	Position         int32
	IsPublished      bool
	SEONoindex       bool
	Metadata         Metadata
	PrivateMetadata  Metadata
	Locale           string // initial content follows
	Name             string
	ShortDescription json.RawMessage
	Description      json.RawMessage
	MetaTitle        string
	MetaDescription  string
	MetaKeywords     string
}

type UpdateCategoryInput struct {
	ID              uuid.UUID
	ParentID        *uuid.UUID // nil = no change - use ClearParent to move to root
	ClearParent     bool       // when true, sets parent_id to NULL
	Slug            *string
	Position        *int32
	IsPublished     *bool
	SEONoindex      *bool
	Metadata        *Metadata
	PrivateMetadata *Metadata
}

type CategoryContentInput struct {
	Locale           string
	Name             string
	ShortDescription json.RawMessage
	Description      json.RawMessage
	MetaTitle        string
	MetaDescription  string
	MetaKeywords     string
}

type RequestCategoryCoverInput struct {
	CategoryID uuid.UUID
	Filename   string
	MimeType   string
	SizeBytes  int64
	AltText    string
}

type RequestCategoryCoverResult struct {
	Category  Category
	UploadURL string
	ExpiresAt time.Time
}

var (
	ErrCategoryNotFound        = errors.New("category not found")
	ErrCategorySlugTaken       = errors.New("category slug already in use")
	ErrCategoryHasChildren     = errors.New("category has subcategories")
	ErrCategoryHasProducts     = errors.New("category has products assigned")
	ErrProductTypeInUse        = errors.New("product type still has products; move or delete them first")
	ErrProductInUse            = errors.New("product is still referenced by other records; remove those references first")
	ErrProductIsChild          = errors.New("product is still a child of a configurable parent; remove it from all parents first")
	ErrCategoryProductNotFound = errors.New("product is not in this category")
	ErrCategoryNoCover         = errors.New("category has no cover image")
	ErrCategoryCoverNotPending = errors.New("category cover is not pending")
)

var ErrCategoryChannelListingNotFound = errors.New("category store listing not found")
