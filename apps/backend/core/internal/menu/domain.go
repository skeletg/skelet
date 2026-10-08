// Package menu manages admin-curated navigation menus - each is a named, slug-addressable tree whose items link either to a catalog, blog or CMS entity, resolved to a canonical URL through url_rewrites at storefront read time, or to a free-form URL
package menu

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Matches urlrewrite.EntityType, so menu items and the storefront catch-all router speak the same vocabulary - an empty TargetType means the item is a free-form URL
type TargetType string

const (
	TargetTypeProduct      TargetType = "product"
	TargetTypeCategory     TargetType = "category"
	TargetTypeCollection   TargetType = "collection"
	TargetTypeBlogPost     TargetType = "blog_post"
	TargetTypeBlogCategory TargetType = "blog_category"
	TargetTypeBlogTag      TargetType = "blog_tag"
	TargetTypeBlogAuthor   TargetType = "blog_author"
	TargetTypeCMSPage      TargetType = "cms_page"
)

// True when t is a known target kind - an empty string is "no target", and the row must then carry a URL, which the DB CHECK enforces
func (t TargetType) IsValid() bool {
	switch t {
	case TargetTypeProduct, TargetTypeCategory, TargetTypeCollection,
		TargetTypeBlogPost, TargetTypeBlogCategory, TargetTypeBlogTag,
		TargetTypeBlogAuthor, TargetTypeCMSPage:
		return true
	}
	return false
}

// Nesting caps at 5 - anything deeper means a UI that can't fit a sane navigation tree on screen, plus rendering pathologies that get noisier per layer
const MaxDepth = 5

var (
	ErrMenuNotFound      = errors.New("menu not found")
	ErrMenuSlugTaken     = errors.New("menu slug already in use")
	ErrMenuItemNotFound  = errors.New("menu item not found")
	ErrInvalidSlug       = errors.New("menu slug must be lowercase letters, digits, hyphens or underscores")
	ErrInvalidTargetType = errors.New("invalid menu item target type")
	ErrTargetXorURL      = errors.New("menu item must specify either a target entity or a URL, not both")
	ErrDepthExceeded     = errors.New("menu nesting exceeds the configured maximum depth")
	ErrParentInOtherMenu = errors.New("menu item parent belongs to a different menu")
	ErrParentCycle       = errors.New("menu item parent would create a cycle")
)

type Menu struct {
	ID         uuid.UUID
	Slug       string
	Name       string
	ItemCount  int64       // populated by List; zero on direct Get calls
	ChannelIDs []uuid.UUID // populated on demand by the service
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type MenuItem struct {
	ID           uuid.UUID
	MenuID       uuid.UUID
	ParentID     *uuid.UUID
	Label        string     // default / fallback when no locale matches
	TargetType   TargetType // empty string when the item is a URL link
	TargetID     *uuid.UUID
	URL          string
	SortOrder    int32
	Translations []MenuItemTranslation // populated on demand by service
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// A locale-specific label override - items without a translation matching the request locale fall back to MenuItem.Label
type MenuItemTranslation struct {
	ItemID    uuid.UUID
	Locale    string
	Label     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateMenuInput struct {
	Slug string
	Name string
}

type UpdateMenuInput struct {
	ID   uuid.UUID
	Slug *string
	Name *string
}

type CreateMenuItemInput struct {
	MenuID     uuid.UUID
	ParentID   *uuid.UUID
	Label      string
	TargetType TargetType // empty for URL items
	TargetID   *uuid.UUID
	URL        string
	SortOrder  int32
}

type UpdateMenuItemInput struct {
	ID          uuid.UUID
	Label       *string
	TargetType  *TargetType
	TargetID    *uuid.UUID
	URL         *string
	ParentID    *uuid.UUID // nil = no change; explicit nil sentinel via ClearParent
	ClearParent bool       // when true, parent_id becomes NULL
	SortOrder   *int32
}

type ReorderMenuItem struct {
	ID        uuid.UUID
	ParentID  *uuid.UUID
	SortOrder int32
}

type Repository interface {
	CreateMenu(ctx context.Context, input CreateMenuInput) (Menu, error)
	GetMenuByID(ctx context.Context, id uuid.UUID) (Menu, error)
	GetMenuBySlug(ctx context.Context, slug string) (Menu, error)
	// Returns a menu only when it's assigned to channelID - used by the storefront resolver, so a menu without an explicit assignment is treated as a draft
	GetMenuBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID) (Menu, error)
	ListMenus(ctx context.Context) ([]Menu, error)
	UpdateMenu(ctx context.Context, input UpdateMenuInput) (Menu, error)
	DeleteMenu(ctx context.Context, id uuid.UUID) error

	ListMenuChannelAssignments(ctx context.Context, menuID uuid.UUID) ([]uuid.UUID, error)
	ListMenuChannelAssignmentsByMenuIDs(ctx context.Context, menuIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	InsertMenuChannelAssignment(ctx context.Context, menuID, channelID uuid.UUID) error
	DeleteMenuChannelAssignment(ctx context.Context, menuID, channelID uuid.UUID) error
	DeleteAllMenuChannelAssignments(ctx context.Context, menuID uuid.UUID) error

	CreateMenuItem(ctx context.Context, input CreateMenuItemInput) (MenuItem, error)
	GetMenuItemByID(ctx context.Context, id uuid.UUID) (MenuItem, error)
	ListMenuItemsByMenu(ctx context.Context, menuID uuid.UUID) ([]MenuItem, error)
	UpdateMenuItem(ctx context.Context, input UpdateMenuItemInput) (MenuItem, error)
	UpdateMenuItemSortOrder(ctx context.Context, id uuid.UUID, sortOrder int32) error
	DeleteMenuItem(ctx context.Context, id uuid.UUID) error

	ListMenuItemTranslations(ctx context.Context, itemID uuid.UUID) ([]MenuItemTranslation, error)
	ListMenuItemTranslationsByItemIDs(ctx context.Context, itemIDs []uuid.UUID) (map[uuid.UUID][]MenuItemTranslation, error)
	UpsertMenuItemTranslation(ctx context.Context, itemID uuid.UUID, locale, label string) (MenuItemTranslation, error)
	DeleteMenuItemTranslation(ctx context.Context, itemID uuid.UUID, locale string) error
}
