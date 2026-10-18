package model

import (
	"time"

	adminmodel "github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
)

type MenuItemTargetType = adminmodel.MenuItemTargetType // re-export keeps front resolvers off a direct admin/model import and keeps gqlgen's front autobind on the right Go type

const (
	MenuItemTargetTypeProduct      = adminmodel.MenuItemTargetTypeProduct
	MenuItemTargetTypeCategory     = adminmodel.MenuItemTargetTypeCategory
	MenuItemTargetTypeCollection   = adminmodel.MenuItemTargetTypeCollection
	MenuItemTargetTypeBlogPost     = adminmodel.MenuItemTargetTypeBlogPost
	MenuItemTargetTypeBlogCategory = adminmodel.MenuItemTargetTypeBlogCategory
	MenuItemTargetTypeBlogTag      = adminmodel.MenuItemTargetTypeBlogTag
	MenuItemTargetTypeBlogAuthor   = adminmodel.MenuItemTargetTypeBlogAuthor
	MenuItemTargetTypeCmsPage      = adminmodel.MenuItemTargetTypeCmsPage
)

// Hand-rolled because the admin MenuItem is flat (parent_id only) and the storefront wants a pre-built `children` tree - pinned in gqlgen.front.yml's `models:` block so gqlgen skips its autobind to admin/model for these names

type Menu struct {
	ID    string      `json:"id"`
	Slug  string      `json:"slug"`
	Name  string      `json:"name"`
	Items []*MenuItem `json:"items"`
}

type MenuItem struct {
	ID         string              `json:"id"`
	ParentID   *string             `json:"parentId,omitempty"`
	Label      string              `json:"label"`
	TargetType *MenuItemTargetType `json:"targetType,omitempty"`
	TargetID   *string             `json:"targetId,omitempty"`
	URL        *string             `json:"url,omitempty"`
	SortOrder  int32               `json:"sortOrder"`
	Children   []*MenuItem         `json:"children"`
	CreatedAt  time.Time           `json:"-"` // kept for symmetry with the admin model - unexposed by the schema, so never serialised
	UpdatedAt  time.Time           `json:"-"`
}
