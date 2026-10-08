package cms

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug: "cms", Name: "CMS", SortOrder: 72,
	})
	permission.Register(permission.Definition{Group: "cms", Slug: "cms.view", Name: "View CMS pages", SortOrder: 1})
	permission.Register(permission.Definition{Group: "cms", Slug: "cms.manage", Name: "Manage CMS pages", SortOrder: 2})
	// Schema editing (page types plus attributes) is gated separately, since adding or removing fields is structurally riskier than editing one page's content
	permission.Register(permission.Definition{Group: "cms", Slug: "cms.types_manage", Name: "Manage CMS page types and attributes", SortOrder: 3})
	// Approving is gated separately from cms.manage so an editor can't approve their own work - publishing still also requires cms.manage
	permission.Register(permission.Definition{Group: "cms", Slug: "cms.approve", Name: "Approve CMS pages for publishing", SortOrder: 4})
}

// A page's editorial status, orthogonal to is_published (the live flag) - literals match the cms_pages.workflow_state CHECK
type WorkflowState string

const (
	WorkflowDraft     WorkflowState = "draft"
	WorkflowInReview  WorkflowState = "in_review"
	WorkflowApproved  WorkflowState = "approved"
	WorkflowPublished WorkflowState = "published"
)

// The kind of value a custom field accepts - the literals match the CHECK on cms_attributes.input_type
type InputType string

const (
	InputText         InputType = "text"
	InputTextarea     InputType = "textarea"
	InputRichText     InputType = "richtext"
	InputNumber       InputType = "number"
	InputBoolean      InputType = "boolean"
	InputDate         InputType = "date"
	InputDateTime     InputType = "datetime"
	InputURL          InputType = "url"
	InputSelect       InputType = "select"
	InputMultiSelect  InputType = "multiselect"
	InputAsset        InputType = "asset"
	InputAssets       InputType = "assets"
	InputRelationship InputType = "relationship"
	InputJSON         InputType = "json"
)

func (t InputType) IsValid() bool {
	switch t {
	case InputText, InputTextarea, InputRichText, InputNumber, InputBoolean,
		InputDate, InputDateTime, InputURL, InputSelect, InputMultiSelect,
		InputAsset, InputAssets, InputRelationship, InputJSON:
		return true
	}
	return false
}

func (t InputType) AcceptsValues() bool {
	return t == InputSelect || t == InputMultiSelect
}

// Whether values of this attribute reference another entity instead of being inline scalars or JSON
func (t InputType) IsRelationship() bool { return t == InputRelationship }

// The kind of entity a relationship attribute can point at - the literals must match the DB CHECKs
type RelationshipTarget string

const (
	TargetCMSPage    RelationshipTarget = "cms_page"
	TargetBlogPost   RelationshipTarget = "blog_post"
	TargetProduct    RelationshipTarget = "product"
	TargetCollection RelationshipTarget = "collection"
	TargetCategory   RelationshipTarget = "category"
)

func (t RelationshipTarget) IsValid() bool {
	switch t {
	case TargetCMSPage, TargetBlogPost, TargetProduct, TargetCollection, TargetCategory:
		return true
	}
	return false
}

// ==================== Attribute ====================

type Attribute struct {
	ID                 uuid.UUID
	Slug               string
	InputType          InputType
	IsTranslatable     bool
	IsRequiredDefault  bool
	Unit               *string
	Validation         json.RawMessage
	DefaultValue       json.RawMessage // typed default seeded into new pages
	RelationshipTarget *RelationshipTarget
	Metadata           map[string]string
	Values             []AttributeValue
	Content            *AttributeContent
	AllContent         []AttributeContent
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type AttributeContent struct {
	Locale      string
	Name        string
	HelperText  *string
	Placeholder *string
	UpdatedAt   time.Time
}

type AttributeValue struct {
	ID          uuid.UUID
	AttributeID uuid.UUID
	Slug        string
	Position    int32
	Content     *AttributeValueContent
	AllContent  []AttributeValueContent
	CreatedAt   time.Time
}

type AttributeValueContent struct {
	Locale    string
	Name      string
	UpdatedAt time.Time
}

type AttributeGroup struct {
	ID         uuid.UUID
	Slug       string
	Position   int32
	Content    *AttributeGroupContent
	AllContent []AttributeGroupContent
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type AttributeGroupContent struct {
	Locale    string
	Name      string
	UpdatedAt time.Time
}

type CreateAttributeInput struct {
	Slug               string
	InputType          InputType
	IsTranslatable     bool
	IsRequiredDefault  bool
	Unit               string
	Validation         json.RawMessage
	DefaultValue       json.RawMessage
	RelationshipTarget *RelationshipTarget
	Metadata           map[string]string
	Values             []AttributeValueInput
}

type AttributeValueInput struct {
	Slug     string
	Position int32
}

type UpdateAttributeInput struct {
	ID                 uuid.UUID
	IsTranslatable     *bool
	IsRequiredDefault  *bool
	Unit               *string
	Validation         json.RawMessage
	DefaultValue       json.RawMessage
	RelationshipTarget *RelationshipTarget
	Metadata           map[string]string
}

type AttributeContentInput struct {
	Locale      string
	Name        string
	HelperText  string
	Placeholder string
}

// ==================== Page Type ====================

type PageType struct {
	ID       uuid.UUID
	Slug     string
	Icon     *string
	IsSystem bool
	Position int32
	// The global URL prefix for pages of this type, "" for bare - per-channel overrides live in ChannelRoots
	RootSlug          string
	ChannelRoots      []PageTypeChannelRoot
	DefaultBlocks     json.RawMessage // starter block Document seeded into new pages
	DefaultMetaTitle  string          // pattern with {title}; inherited when a page's meta title is blank
	DefaultSEONoindex bool            // inherited unless the page sets its own
	IsSingleton       bool            // at most one page of this type may exist
	Content           *PageTypeContent
	AllContent        []PageTypeContent
	Attributes        []PageTypeAttribute
	Groups            []AttributeGroup
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PageTypeChannelRoot struct {
	ChannelID uuid.UUID
	RootSlug  string
}

// The channel override if present, else the type's global RootSlug - a nil channelID means global
func (pt PageType) EffectiveRoot(channelID *uuid.UUID) string {
	if channelID != nil {
		for _, cr := range pt.ChannelRoots {
			if cr.ChannelID == *channelID {
				return cr.RootSlug
			}
		}
	}
	return pt.RootSlug
}

type PageTypeContent struct {
	Locale      string
	Name        string
	Description *string
	UpdatedAt   time.Time
}

type PageTypeAttribute struct {
	Attribute  Attribute
	GroupID    *uuid.UUID
	Position   int32
	IsRequired bool
	// Conditional visibility - show this attribute only when the sibling ConditionAttributeID holds ConditionValue - Nil is always shown
	ConditionAttributeID *uuid.UUID
	ConditionValue       *string
}

type CreatePageTypeInput struct {
	Slug        string
	Icon        string
	IsSystem    bool
	Position    int32
	Locale      string
	Name        string
	Description string
}

type UpdatePageTypeInput struct {
	ID       uuid.UUID
	Icon     *string
	Position *int32
}

// The structural fields edited together on the page-type form - pointers, so an omitted field is left unchanged, and changing RootSlug triggers a re-sync of all the type's page URLs
type UpdatePageTypeStructureInput struct {
	ID                uuid.UUID
	RootSlug          *string
	DefaultMetaTitle  *string
	DefaultSEONoindex *bool
	IsSingleton       *bool
}

// The fully-merged structural values the repo writes, after the service folds a partial UpdatePageTypeStructureInput over the current row
type ResolvedPageTypeStructure struct {
	ID                uuid.UUID
	RootSlug          string
	DefaultMetaTitle  string
	DefaultSEONoindex bool
	IsSingleton       bool
}

type PageTypeContentInput struct {
	Locale      string
	Name        string
	Description string
}

type AssignAttributeInput struct {
	PageTypeID           uuid.UUID
	AttributeID          uuid.UUID
	GroupID              *uuid.UUID
	Position             int32
	IsRequired           bool
	ConditionAttributeID *uuid.UUID
	ConditionValue       *string
}

// ==================== Page ====================

type Page struct {
	ID              uuid.UUID
	PageTypeID      uuid.UUID
	ParentID        *uuid.UUID
	Slug            string
	BasePath        string // hierarchical slug path, no type root prefix
	URLPath         string // public/global path = joinRoot(type root, BasePath)
	Position        int32
	IsPublished     bool
	SEONoindex      bool
	PublishedAt     *time.Time
	ScheduledAt     *time.Time      // future go-live; cms-publish job flips IsPublished
	Data            json.RawMessage // non-translated custom field bag
	Metadata        map[string]string
	PrivateMetadata map[string]string
	// Free-form labels used for filtering in admin lists and the page-type table, and for future storefront querying - always non-nil
	Tags []string
	// Editorial workflow, orthogonal to IsPublished - the state plus actor and time stamps for submit and approve, with AssignedTo the reviewer
	WorkflowState WorkflowState
	AssignedTo    *uuid.UUID
	SubmittedBy   *uuid.UUID
	SubmittedAt   *time.Time
	ApprovedBy    *uuid.UUID
	ApprovedAt    *time.Time
	// Binds this page to a Prism experiment for A/B testing, "" when not under test
	// Variants holds the treatment bodies for the active locale, keyed by the experiment's variation keys, while the CONTROL variant is Content.Body - populated by the front resolver for the rendered locale
	ExperimentKey   string
	Variants        []PageVariant
	Content         *PageContent
	AllContent      []PageContent
	Selections      []PageAttributeSelection
	Relationships   []PageRelationship
	ChannelListings []PageChannelListing
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// One A/B treatment body for a (page, locale), keyed by the bound experiment's variation key - the control body lives on PageContent
type PageVariant struct {
	VariantKey string
	Body       json.RawMessage
}

// Controls whether a globally-published page is exposed on a particular store - storefront queries filter by these rows while admin queries return them all so the editor can show per-store status - Mirrors blog.PostChannelListing
type PageChannelListing struct {
	ChannelID   uuid.UUID
	IsPublished bool
	SEONoindex  bool
	// Marks this page as the channel's homepage, served at "/" - at most one page per channel may set it, DB-enforced
	IsHomepage bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type SetPageChannelListingInput struct {
	PageID      uuid.UUID
	ChannelID   uuid.UUID
	IsPublished bool
	SEONoindex  bool
	IsHomepage  bool
}

type PageContent struct {
	Locale          string
	Title           string
	Body            json.RawMessage
	MetaTitle       *string
	MetaDescription *string
	MetaKeywords    *string
	Data            json.RawMessage // translated custom field bag
	UpdatedAt       time.Time
}

type PageAttributeSelection struct {
	AttributeID uuid.UUID
	ValueID     uuid.UUID
}

type PageRelationship struct {
	AttributeID uuid.UUID
	TargetType  RelationshipTarget
	TargetID    uuid.UUID
	Position    int32
}

type CreatePageInput struct {
	PageTypeID      uuid.UUID
	ParentID        *uuid.UUID
	Slug            string
	Position        int32
	SEONoindex      bool
	Data            json.RawMessage
	Metadata        map[string]string
	PrivateMetadata map[string]string
	Locale          string
	Title           string
	Body            json.RawMessage
	MetaTitle       string
	MetaDescription string
	MetaKeywords    string
	ContentData     json.RawMessage
	Selections      []PageAttributeSelection
	Relationships   []PageRelationship
}

type UpdatePageInput struct {
	ID              uuid.UUID
	ParentID        *uuid.UUID
	ClearParent     bool
	Slug            *string
	Position        *int32
	SEONoindex      *bool
	Data            json.RawMessage
	Metadata        map[string]string
	PrivateMetadata map[string]string
	// Replaces the page's tag set when non-nil - nil leaves it unchanged, and an empty non-nil slice clears all tags
	Tags *[]string
}

type PageContentInput struct {
	Locale          string
	Title           string
	Body            json.RawMessage
	MetaTitle       string
	MetaDescription string
	MetaKeywords    string
	Data            json.RawMessage
}

type PageListFilter struct {
	PageTypeID *uuid.UUID
	ParentID   *uuid.UUID
	// When set, restricts results to pages with an active listing on that store - storefront paths set it from the active store context, admin paths leave it nil to see everything
	// When nil, OnlyPublished still applies to the global cms_pages flag
	ChannelID     *uuid.UUID
	OnlyPublished bool
	// When set, restricts to pages in that editorial state, e.g. "in_review" for the review queue - admin only
	WorkflowState *string
	// When non-empty, switches to ranked full-text search over the requested locale's content - title, meta and block body - Admin only
	Search string
	Limit  int32
	Offset int32
}

// ==================== Errors ====================

var (
	ErrAttributeNotFound      = errors.New("cms attribute not found")
	ErrAttributeValueNotFound = errors.New("cms attribute value not found")
	ErrAttributeGroupNotFound = errors.New("cms attribute group not found")
	ErrPageTypeNotFound       = errors.New("cms page type not found")
	ErrPageNotFound           = errors.New("cms page not found")
	ErrPageContentNotFound    = errors.New("cms page content not found for this locale")
	ErrSlugTaken              = errors.New("slug already in use")
	ErrInvalidSlug            = errors.New("invalid slug")
	ErrInvalidInputType       = errors.New("invalid input type")
	ErrInvalidRelationship    = errors.New("invalid relationship target")
	ErrValuesNotApplicable    = errors.New("attribute does not accept select values")
	ErrAttributeInUse         = errors.New("attribute is assigned to one or more page types")
	ErrPageTypeHasPages       = errors.New("page type has pages")
	ErrPageTypeIsSystem       = errors.New("system page types cannot be deleted")
	ErrSingletonExists        = errors.New("a page of this singleton type already exists")
	ErrApprovalRequired       = errors.New("page must be approved before it can be published")
	ErrRequiredFieldMissing   = errors.New("required field is missing")
	ErrInvalidFieldValue      = errors.New("invalid field value")
	ErrURLPathConflict        = errors.New("url path already in use")
	ErrSnippetNotFound        = errors.New("cms snippet not found")
	ErrBlockTypeNotFound      = errors.New("cms block type not found")
	ErrPageCycle              = errors.New("a page cannot be nested under itself or its own descendant")
	ErrPageTooDeep            = errors.New("page nesting is too deep")
	ErrPageHasChildren        = errors.New("page has child pages")
	ErrSelfApproval           = errors.New("a page cannot be approved by the admin who submitted it")
	ErrNotInReview            = errors.New("page is not in review")
	ErrAttributeValueInUse    = errors.New("attribute value is selected on one or more pages")
	ErrRevisionNotFound       = errors.New("cms page revision not found")
	ErrCommentNotFound        = errors.New("cms page comment not found")
	ErrChannelNotFound        = errors.New("channel not found")
	ErrAdminRequired          = errors.New("an authenticated admin is required")
	ErrReservedBlockKey       = errors.New("block type key is reserved by a built-in block")
	ErrTagNotFound            = errors.New("cms tag not found")
)

// A content-version snapshot of a (page, locale) written on every content save - restoring one re-applies its title, body, meta and data
type PageRevision struct {
	ID              uuid.UUID
	PageID          uuid.UUID
	Locale          string
	Title           string
	Body            json.RawMessage
	MetaTitle       *string
	MetaDescription *string
	MetaKeywords    *string
	Data            json.RawMessage
	AuthorAdminID   *uuid.UUID
	CreatedAt       time.Time
}

// A flat collaboration note on a page, optionally anchored to a locale and/or a specific block id, which is the dashboard's click-to-edit selection
type PageComment struct {
	ID         uuid.UUID
	PageID     uuid.UUID
	Locale     *string
	BlockID    *string
	AuthorID   uuid.UUID
	Body       string
	Resolved   bool
	ResolvedBy *uuid.UUID
	ResolvedAt *time.Time
	CreatedAt  time.Time
}

type CommentInput struct {
	PageID  uuid.UUID
	Locale  *string
	BlockID *string
	Body    string
}

// An entry in the managed tag vocabulary - pages reference a tag by its stable Slug stored in cms_pages.tags, while Name is the editable display label
type Tag struct {
	Slug      string
	Name      string
	CreatedAt time.Time
}

// A reusable section - a saved block subtree stored as a block Document - Pages embed it with a `ref` block and the front resolver expands it server-side, so editing the snippet updates every page
type Snippet struct {
	ID        uuid.UUID
	Slug      string
	Name      string
	Blocks    json.RawMessage // a { version, blocks } Document
	CreatedAt time.Time
	UpdatedAt time.Time
}

// An admin-defined custom block - a field schema plus an HTML template - Instances of type Key render by interpolating their props into the template and sanitizing - Global, since CMS pages are global
type BlockType struct {
	ID           uuid.UUID
	Key          string
	Name         string
	Icon         string
	Fields       json.RawMessage // field-schema JSON (editor descriptor)
	HTMLTemplate string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// No shared Repository interface - each sub-service declares the slice it uses (attributeStore, pageStore and the rest)
