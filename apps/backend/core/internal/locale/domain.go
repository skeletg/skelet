// Package locale manages first-class locales - BCP 47 codes registered once and referenced by FK from every other locale-bearing column
// The shape mirrors internal/channel: a small Service over a Repository, plus an init() registering the locales.view and locales.manage permissions
package locale

import (
	"context"
	"errors"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "locales",
		Name:      "Locales",
		SortOrder: 65,
	})
	permission.Register(permission.Definition{
		Group: "locales",
		Slug:  "locales.view",
		Name:  "View locales",
	})
	permission.Register(permission.Definition{
		Group: "locales",
		Slug:  "locales.manage",
		Name:  "Manage locales",
	})
}

// One BCP 47 language tag with display and presentation hints - Code is the natural key, and renaming one is intentionally hard: delete-then-recreate, which the RESTRICT delete guard refuses while references exist
// Every extra string field uses the empty-string sentinel for "use the storefront library's default" - the backend never parses formatting strings or country codes beyond shape validation
type Locale struct {
	Code       string
	Name       string
	NativeName string
	IsActive   bool
	IsRTL      bool
	SortOrder  int32

	DateFormat         string // e.g. "DD.MM.YYYY"
	TimeFormat         string // e.g. "HH:mm"
	DecimalSeparator   string // "." or ","
	ThousandsSeparator string
	FirstDayOfWeek     int16 // 0=Sunday … 6=Saturday

	DefaultCountryCode string // ISO 3166-1 alpha-2 (uppercase) or ""
	FlagEmoji          string // e.g. "🇫🇷"
	HreflangValue      string // SEO; lowercased; "" → fall back to Code at render time

	Script                string // ISO 15924 (e.g. "Latn", "Cyrl")
	PluralRule            string // free-text; consumer decides what to do with it
	EmailFromNameOverride string

	Description string
	Metadata    map[string]string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateLocaleInput struct {
	Code       string
	Name       string
	NativeName string
	IsActive   bool
	IsRTL      bool
	SortOrder  int32

	DateFormat         string
	TimeFormat         string
	DecimalSeparator   string
	ThousandsSeparator string
	FirstDayOfWeek     int16

	DefaultCountryCode string
	FlagEmoji          string
	HreflangValue      string

	Script                string
	PluralRule            string
	EmailFromNameOverride string

	Description string
	Metadata    map[string]string
}

// Pointer fields for partial updates - Code is the PK and never updates, so "renaming" a locale means delete and recreate
type UpdateLocaleInput struct {
	Code       string
	Name       *string
	NativeName *string
	IsActive   *bool
	IsRTL      *bool
	SortOrder  *int32

	DateFormat         *string
	TimeFormat         *string
	DecimalSeparator   *string
	ThousandsSeparator *string
	FirstDayOfWeek     *int16

	DefaultCountryCode *string
	FlagEmoji          *string
	HreflangValue      *string

	Script                *string
	PluralRule            *string
	EmailFromNameOverride *string

	Description *string
	Metadata    *map[string]string
}

var (
	ErrLocaleNotFound  = errors.New("locale not found")
	ErrLocaleCodeTaken = errors.New("locale code already exists")
	ErrLocaleInUse     = errors.New("locale has references; cannot delete")
	ErrInvalidCode     = errors.New("invalid BCP 47 locale code")
)

type Repository interface {
	Create(ctx context.Context, input CreateLocaleInput) (Locale, error)
	GetByCode(ctx context.Context, code string) (Locale, error)
	List(ctx context.Context, activeOnly bool) ([]Locale, error)
	Update(ctx context.Context, input UpdateLocaleInput) (Locale, error)
	Delete(ctx context.Context, code string) error
	CountReferences(ctx context.Context, code string) (int64, error)
}
