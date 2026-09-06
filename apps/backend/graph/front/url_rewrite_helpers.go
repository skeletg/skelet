package front // url_rewrite.resolvers.go helpers live here so gqlgen's quarantine-non-resolver-helpers pass can't bury them in a comment block, same reason blog_helpers.go exists

import (
	"github.com/skeletg/skelet/apps/backend/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
)

// Unknown entity_types - plugins and microservices registering their own via the admin syncCanonicalUrlRewrite mutation - collapse to RouteKindPlugin, and the raw string surfaces in RouteResolution.pluginKind for the storefront to switch on
func routeKindFromEntity(t urlrewrite.EntityType) model.RouteKind {
	switch t {
	case urlrewrite.EntityTypeProduct:
		return model.RouteKindProduct
	case urlrewrite.EntityTypeCategory:
		return model.RouteKindCategory
	case urlrewrite.EntityTypeCollection:
		return model.RouteKindCollection
	case urlrewrite.EntityTypeBlogPost:
		return model.RouteKindBlogPost
	case urlrewrite.EntityTypeBlogCategory:
		return model.RouteKindBlogCategory
	case urlrewrite.EntityTypeBlogTag:
		return model.RouteKindBlogTag
	case urlrewrite.EntityTypeBlogAuthor:
		return model.RouteKindBlogAuthor
	case urlrewrite.EntityTypeCMSPage:
		return model.RouteKindCmsPage
	}
	return model.RouteKindPlugin
}

func pluginKindFor(t urlrewrite.EntityType) *string { // raw entity_type slug for plugin-registered entities, nil for core kinds so the GraphQL output stays tidy
	switch t {
	case urlrewrite.EntityTypeProduct,
		urlrewrite.EntityTypeCategory,
		urlrewrite.EntityTypeCollection,
		urlrewrite.EntityTypeBlogPost,
		urlrewrite.EntityTypeBlogCategory,
		urlrewrite.EntityTypeBlogTag,
		urlrewrite.EntityTypeBlogAuthor,
		urlrewrite.EntityTypeCMSPage:
		return nil
	}
	s := string(t)
	return &s
}
