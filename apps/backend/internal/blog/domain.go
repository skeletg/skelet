package blog

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/permission"
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "blog",
		Name:      "Blog",
		SortOrder: 71,
	})
	permission.Register(permission.Definition{
		Group: "blog", Slug: "blog.view", Name: "View Blog", SortOrder: 1,
	})
	permission.Register(permission.Definition{
		Group: "blog", Slug: "blog.manage", Name: "Manage Blog", SortOrder: 2,
	})
}

type PostStatus string

const (
	PostStatusDraft     PostStatus = "draft"
	PostStatusScheduled PostStatus = "scheduled"
	PostStatusPublished PostStatus = "published"
	PostStatusArchived  PostStatus = "archived"
)

func (s PostStatus) IsValid() bool {
	switch s {
	case PostStatusDraft, PostStatusScheduled, PostStatusPublished, PostStatusArchived:
		return true
	}
	return false
}

type Metadata map[string]string // flat and persisted as JSONB, mirroring the catalog domain's `Metadata` so admin clients can reuse editors across surfaces

// ==================== Author ====================

type Author struct {
	ID            uuid.UUID
	Slug          string
	DisplayName   string
	Email         *string
	AvatarAssetID *uuid.UUID
	LinkedAdminID *uuid.UUID
	Metadata      Metadata
	Content       *AuthorContent
	AllContent    []AuthorContent
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type AuthorContent struct {
	Locale    string
	Name      string
	Bio       json.RawMessage
	UpdatedAt time.Time
}

type CreateAuthorInput struct {
	Slug          string
	DisplayName   string
	Email         *string
	AvatarAssetID *uuid.UUID
	LinkedAdminID *uuid.UUID
	Metadata      Metadata
	Locale        string // optional initial localized content
	Name          string
	Bio           json.RawMessage
}

type UpdateAuthorInput struct {
	ID            uuid.UUID
	Slug          *string
	DisplayName   *string
	Email         *string
	ClearEmail    bool
	AvatarAssetID *uuid.UUID
	ClearAvatar   bool
	LinkedAdminID *uuid.UUID
	ClearAdmin    bool
	Metadata      *Metadata
}

type AuthorContentInput struct {
	Locale string
	Name   string
	Bio    json.RawMessage
}

// ==================== Category ====================

type Category struct {
	ID          uuid.UUID
	ParentID    *uuid.UUID
	Slug        string
	URLPath     string
	Position    int32
	IsPublished bool
	SEONoindex  bool
	Content     *CategoryContent
	AllContent  []CategoryContent
	Children    []Category
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CategoryContent struct {
	Locale          string
	Name            string
	Description     json.RawMessage
	MetaTitle       *string
	MetaDescription *string
	MetaKeywords    *string
	UpdatedAt       time.Time
}

type CreateCategoryInput struct {
	ParentID        *uuid.UUID
	Slug            string
	Position        int32
	IsPublished     bool
	SEONoindex      bool
	Locale          string // initial localized content, required at creation time
	Name            string
	Description     json.RawMessage
	MetaTitle       string
	MetaDescription string
	MetaKeywords    string
}

type UpdateCategoryInput struct {
	ID          uuid.UUID
	ParentID    *uuid.UUID
	ClearParent bool
	Slug        *string
	Position    *int32
	IsPublished *bool
	SEONoindex  *bool
}

type CategoryContentInput struct {
	Locale          string
	Name            string
	Description     json.RawMessage
	MetaTitle       string
	MetaDescription string
	MetaKeywords    string
}

// ==================== Tag ====================

type Tag struct {
	ID          uuid.UUID
	Slug        string
	IsPublished bool
	Content     *TagContent
	AllContent  []TagContent
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TagContent struct {
	Locale          string
	Name            string
	Description     json.RawMessage
	MetaTitle       *string
	MetaDescription *string
	MetaKeywords    *string
	UpdatedAt       time.Time
}

type CreateTagInput struct {
	Slug            string
	IsPublished     bool
	Locale          string // initial localized content
	Name            string
	Description     json.RawMessage
	MetaTitle       string
	MetaDescription string
	MetaKeywords    string
}

type UpdateTagInput struct {
	ID          uuid.UUID
	Slug        *string
	IsPublished *bool
}

type TagContentInput struct {
	Locale          string
	Name            string
	Description     json.RawMessage
	MetaTitle       string
	MetaDescription string
	MetaKeywords    string
}

// ==================== Post ====================

type Post struct {
	ID                 uuid.UUID
	Slug               string
	AuthorID           *uuid.UUID
	CoverAssetID       *uuid.UUID
	Status             PostStatus
	PublishedAt        *time.Time
	SEONoindex         bool
	ReadingTimeMinutes *int32
	Metadata           Metadata
	PrivateMetadata    Metadata
	Content            *PostContent
	AllContent         []PostContent
	Categories         []Category
	Tags               []Tag
	Author             *Author
	ChannelListings    []PostChannelListing
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type PostChannelListing struct { // whether a globally-published post is exposed on a particular store - public storefront queries filter by these rows
	ChannelID   uuid.UUID
	IsPublished bool
	SEONoindex  bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SetPostChannelListingInput struct {
	PostID      uuid.UUID
	ChannelID   uuid.UUID
	IsPublished bool
	SEONoindex  bool
}

type PostContent struct {
	Locale          string
	Title           string
	Excerpt         json.RawMessage
	Body            json.RawMessage
	MetaTitle       *string
	MetaDescription *string
	MetaKeywords    *string
	UpdatedAt       time.Time
}

type CreatePostInput struct {
	Slug               string
	AuthorID           *uuid.UUID
	CoverAssetID       *uuid.UUID
	Status             PostStatus
	PublishedAt        *time.Time
	SEONoindex         bool
	ReadingTimeMinutes *int32
	Metadata           Metadata
	PrivateMetadata    Metadata
	Locale             string // initial localized content, required at creation time
	Title              string
	Excerpt            json.RawMessage
	Body               json.RawMessage
	MetaTitle          string
	MetaDescription    string
	MetaKeywords       string
	CategoryIDs        []uuid.UUID // category/tag attachments are optional
	TagIDs             []uuid.UUID
}

type UpdatePostInput struct {
	ID                 uuid.UUID
	Slug               *string
	AuthorID           *uuid.UUID
	ClearAuthor        bool
	CoverAssetID       *uuid.UUID
	ClearCover         bool
	SEONoindex         *bool
	ReadingTimeMinutes *int32
	Metadata           *Metadata
	PrivateMetadata    *Metadata
}

type PostContentInput struct {
	Locale          string
	Title           string
	Excerpt         json.RawMessage
	Body            json.RawMessage
	MetaTitle       string
	MetaDescription string
	MetaKeywords    string
}

type PostListFilter struct { // every field optional - OnlyPublished is the storefront default and admin queries opt into drafts/archived, while a set ChannelID restricts results to posts with an active listing on that store, which admin queries leave nil to see posts globally
	OnlyPublished bool
	ChannelID     *uuid.UUID
	CategoryID    *uuid.UUID
	TagID         *uuid.UUID
	AuthorID      *uuid.UUID
	Status        *PostStatus
	Limit         int32
	Offset        int32
}

// ==================== Errors ====================

var (
	ErrAuthorNotFound          = errors.New("blog author not found")
	ErrAuthorContentNotFound   = errors.New("blog author content not found for this locale")
	ErrCategoryNotFound        = errors.New("blog category not found")
	ErrCategoryContentNotFound = errors.New("blog category content not found for this locale")
	ErrCategoryHasChildren     = errors.New("blog category has subcategories")
	ErrCategoryHasPosts        = errors.New("blog category has posts assigned")
	ErrTagNotFound             = errors.New("blog tag not found")
	ErrTagContentNotFound      = errors.New("blog tag content not found for this locale")
	ErrPostNotFound            = errors.New("blog post not found")
	ErrPostContentNotFound     = errors.New("blog post content not found for this locale")
	ErrSlugTaken               = errors.New("slug already in use")
	ErrInvalidSlug             = errors.New("invalid slug")
	ErrInvalidStatus           = errors.New("invalid post status")
	ErrPublishedAtRequired     = errors.New("published_at is required for scheduled and published posts")
	ErrScheduledInPast         = errors.New("scheduled publish time must be in the future")
)

// The persistence layer is the concrete *postgresRepository in repository.go - there is deliberately no producer-defined Repository interface, since it had exactly one consumer (this package's Service) and one implementation, so it bought no abstraction and only an 80-method surface to keep in sync; removed alongside the same cleanup in internal/catalog and internal/cms
