// Package review owns the product-review domain - an operator-curated question library, per-product-type assignments, customer reviews with per-question answers and media, and an admin moderation queue
// Question prompts localize through review_question_content, a sparse PK on (question_id, locale); customer-authored answer bodies don't
package review

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/permission"
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "reviews",
		Name:      "Reviews",
		SortOrder: 75,
	})
	permission.Register(permission.Definition{
		Group: "reviews", Slug: "reviews.view", Name: "View Reviews", SortOrder: 1,
	})
	permission.Register(permission.Definition{
		Group: "reviews", Slug: "reviews.manage", Name: "Manage Review Questions", SortOrder: 2,
	})
	permission.Register(permission.Definition{
		Group: "reviews", Slug: "reviews.moderate", Name: "Moderate Reviews",
		Description: "Approve, reject and edit customer reviews before they reach the storefront. Separate from reviews.manage so a moderator can clear the queue without reaching review settings or questions.",
		SortOrder:   3,
	})
}

// The kind of answer a question asks for - TEXT with a CHECK in Postgres
type InputType string

const (
	InputTypeRating1To5 InputType = "rating_1_5"
	InputTypeYesNo      InputType = "yes_no"
	InputTypeFreeText   InputType = "free_text"
)

func (t InputType) IsValid() bool {
	switch t {
	case InputTypeRating1To5, InputTypeYesNo, InputTypeFreeText:
		return true
	}
	return false
}

// A review's moderation state - new reviews land 'pending' and only 'approved' surfaces on the storefront
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusApproved, StatusRejected:
		return true
	}
	return false
}

// One operator-curated question assigned to product types - the locale-sensitive label and help_text live in QuestionContent, the rest is invariant
type Question struct {
	ID           uuid.UUID
	Slug         string
	InputType    InputType
	IsRequired   bool
	AllowComment bool
	SortOrder    int32
	// Content is the prompt for one requested locale and AllContent holds every translation - both can be empty on a raw row read
	Content    *QuestionContent
	AllContent []QuestionContent
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type QuestionContent struct {
	QuestionID uuid.UUID
	Locale     string
	Label      string
	HelpText   string
	UpdatedAt  time.Time
}

// One (product_type, question) junction row - SortOrder orders the questions in the storefront review form
type Assignment struct {
	ProductTypeID uuid.UUID
	QuestionID    uuid.UUID
	SortOrder     int32
}

// The customer-authored review header, with answers and media loaded separately - the author is exactly one of CustomerID or GuestName, enforced by reviews_author_present_chk
type Review struct {
	ID         uuid.UUID
	ProductID  uuid.UUID
	CustomerID *uuid.UUID
	// Guest submissions only - GuestEmail is admin-visible and never reaches the storefront
	GuestName          string
	GuestEmail         string
	ChannelID          uuid.UUID
	Locale             string
	Title              string
	Body               string
	IsVerifiedPurchase bool
	Status             Status
	SubmittedAt        time.Time
	ModeratedAt        *time.Time
	ModeratedBy        *uuid.UUID
	ModeratorNote      string
	// The AI moderation verdict, a suggestion that never changes Status - a coarse flag where "" is unchecked, then "genuine" or "flagged"
	ModerationLabel     string
	ModerationReason    string
	ModerationCheckedAt *time.Time
	// AI-assigned or moderator-overridden topic slugs, empty meaning uncategorized - a review can carry several, set in the same pass as moderation
	CategorySlugs []string
	// AI-assigned or overridden concern slugs like profanity and spam, empty for none
	ModerationTags  []string
	MerchantReply   string
	MerchantReplyAt *time.Time
	MerchantReplyBy *uuid.UUID
	// Answers and Media are hydrated by GetReviewWithDetails or the front resolver, empty on a raw row
	Answers []Answer
	Media   []Media
}

// One (review, question) cell - which of RatingValue, BoolValue or TextValue is meaningful depends on InputType, though comment-allowing questions carry text too
type Answer struct {
	ReviewID    uuid.UUID
	QuestionID  uuid.UUID
	RatingValue *int16
	BoolValue   *bool
	TextValue   string
}

// One review attachment - ObjectKey and MimeType are denormalized from the media asset for cheap reads
type Media struct {
	ID        uuid.UUID
	ReviewID  uuid.UUID
	AssetID   uuid.UUID
	SortOrder int32
	CreatedAt time.Time
	ObjectKey string
	MimeType  string
	Width     *int32
	Height    *int32
	SizeBytes int64
}

type CreateQuestionInput struct {
	Slug         string
	InputType    InputType
	IsRequired   bool
	AllowComment bool
	SortOrder    int32
}

type UpdateQuestionInput struct {
	ID           uuid.UUID
	IsRequired   bool
	AllowComment bool
	SortOrder    int32
}

type QuestionContentInput struct {
	QuestionID uuid.UUID
	Locale     string
	Label      string
	HelpText   string
}

// An operator-curated topic the AI classifies reviews into, separate from the moderation label - global, and the Description steers the classifier
type Category struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Description string
	IsActive    bool
	SortOrder   int32
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateCategoryInput struct {
	Slug        string
	Name        string
	Description string
	IsActive    bool
	SortOrder   int32
}

type UpdateCategoryInput struct {
	ID          uuid.UUID
	Name        string
	Description string
	IsActive    bool
	SortOrder   int32
}

// An operator-curated moderation concern the AI can flag - profanity, off-topic, spam - Global, several per review, and the Description steers the classifier
type ModerationTag struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Description string
	IsActive    bool
	SortOrder   int32
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateModerationTagInput struct {
	Slug        string
	Name        string
	Description string
	IsActive    bool
	SortOrder   int32
}

type UpdateModerationTagInput struct {
	ID          uuid.UUID
	Name        string
	Description string
	IsActive    bool
	SortOrder   int32
}

type AnswerInput struct {
	QuestionID  uuid.UUID
	RatingValue *int16
	BoolValue   *bool
	TextValue   string
}

type SubmitReviewInput struct {
	ProductID uuid.UUID
	// Exactly one of CustomerID or GuestName - a nil CustomerID under require_authentication=true is rejected with ErrGuestNotAllowed
	CustomerID    *uuid.UUID
	GuestName     string
	GuestEmail    string
	ChannelID     uuid.UUID
	Locale        string
	Title         string
	Body          string
	Answers       []AnswerInput
	MediaAssetIDs []uuid.UUID
}

type ListReviewsFilter struct {
	Status    []Status
	ProductID *uuid.UUID
	// Scopes to an admin's permitted channels - nil is no filter while a non-nil, even empty, slice restricts, so no permitted channels yields an empty page
	// Set from the resolver's PermittedChannels, never from client input
	ChannelIDs []uuid.UUID
	// Filters to reviews in any of these categories, client-supplied, with nil or empty meaning no filter
	CategorySlugs []string
}

// The storefront review summary card - count, overall mean, the 5-bucket distribution and per-question means
type Aggregates struct {
	Count          int
	OverallAverage *float64
	// Distribution[i] counts rating_1_5 answers equal to i+1, so [0] is 1 star and [4] is 5 - always 5 entries
	Distribution [5]int
	PerQuestion  []QuestionAverage
}

type QuestionAverage struct {
	QuestionID uuid.UUID
	Average    float64
	Count      int
}

// The cached AI "what customers say" summary, served verbatim with no AI call at render
type ProductReviewSummary struct {
	ProductID   uuid.UUID
	Summary     string
	ReviewCount int
	IsStale     bool
	GeneratedAt *time.Time
	UpdatedAt   time.Time
}

var (
	ErrNotFound          = errors.New("review: not found")
	ErrQuestionNotFound  = errors.New("review: question not found")
	ErrCategoryNotFound  = errors.New("review: category not found")
	ErrTagNotFound       = errors.New("review: moderation tag not found")
	ErrSlugTaken         = errors.New("review: slug already exists")
	ErrInvalidInputType  = errors.New("review: invalid input type")
	ErrInvalidStatus     = errors.New("review: invalid status")
	ErrAlreadyReviewed   = errors.New("review: customer already reviewed this product")
	ErrNotPurchased      = errors.New("review: customer has not purchased this product")
	ErrGuestNotAllowed   = errors.New("review: guest submissions are disabled")
	ErrGuestNameRequired = errors.New("review: guest reviewers must supply a name")
	ErrAnswerOutOfRange  = errors.New("review: rating must be between 1 and 5")
	ErrMissingRequired   = errors.New("review: a required question is missing an answer")
	ErrUnknownQuestion   = errors.New("review: answer references a question not assigned to this product type")
	ErrInvalidVote       = errors.New("review: vote value must be -1, 0, or 1")
	ErrInvalidVoter      = errors.New("review: vote requires exactly one of customer or anon token")
	ErrVoteAuthRequired  = errors.New("review: voting is restricted to signed-in customers")
	ErrFieldTooLong      = errors.New("review: a text field exceeds its maximum length")
)

// Server-side length caps, the backstop against a client bypassing the storefront form's shorter limits and abusing storage with oversized payloads
const (
	MaxReviewTitleLen = 200
	MaxReviewBodyLen  = 5000
	MaxGuestNameLen   = 120
	MaxGuestEmailLen  = 254
)

// Enforces the caps above - pure, so it's unit-tested directly, and it counts bytes, a generous superset of the form's char limits
func validateFieldLengths(in SubmitReviewInput) error {
	if len(in.Title) > MaxReviewTitleLen ||
		len(in.Body) > MaxReviewBodyLen ||
		len(in.GuestName) > MaxGuestNameLen ||
		len(in.GuestEmail) > MaxGuestEmailLen {
		return ErrFieldTooLong
	}
	return nil
}

type VoteCounts struct {
	Helpful    int
	NotHelpful int
}

// A vote's identity, exactly one of CustomerID or AnonToken or else ErrInvalidVoter - AnonToken is a storefront cookie, not authenticated, just stable enough to debounce double-clicks from one browser
type Voter struct {
	CustomerID *uuid.UUID
	AnonToken  *uuid.UUID
}

func (v Voter) IsAnon() bool      { return v.CustomerID == nil && v.AnonToken != nil }
func (v Voter) IsCustomer() bool  { return v.CustomerID != nil && v.AnonToken == nil }
func (v Voter) IsValid() bool     { return v.IsAnon() != v.IsCustomer() }
func (v Voter) IsAnonymous() bool { return v.IsAnon() }

// The review persistence boundary, returning domain types rather than DB rows - service callers compose these into transactions through internal/tx.Manager
