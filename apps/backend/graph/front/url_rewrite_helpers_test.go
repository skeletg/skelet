package front

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
)

func TestRouteKindCoversEveryCoreEntity(t *testing.T) { // the storefront routes on this switch, so a core entity falling through to PLUGIN would 404 every URL of that kind
	for entity, want := range map[urlrewrite.EntityType]model.RouteKind{
		urlrewrite.EntityTypeProduct:      model.RouteKindProduct,
		urlrewrite.EntityTypeCategory:     model.RouteKindCategory,
		urlrewrite.EntityTypeCollection:   model.RouteKindCollection,
		urlrewrite.EntityTypeBlogPost:     model.RouteKindBlogPost,
		urlrewrite.EntityTypeBlogCategory: model.RouteKindBlogCategory,
		urlrewrite.EntityTypeBlogTag:      model.RouteKindBlogTag,
		urlrewrite.EntityTypeBlogAuthor:   model.RouteKindBlogAuthor,
		urlrewrite.EntityTypeCMSPage:      model.RouteKindCmsPage,
	} {
		if got := routeKindFromEntity(entity); got != want {
			t.Errorf("%s -> %s, want %s", entity, got, want)
		}
		// core kinds carry no pluginKind, so the storefront's plugin switch never sees them
		if pk := pluginKindFor(entity); pk != nil {
			t.Errorf("pluginKindFor(%s) = %q, want nil for a core kind", entity, *pk)
		}
	}
}

func TestUnknownEntityCollapsesToPlugin(t *testing.T) { // a plugin's registered entity_type must survive verbatim in pluginKind - it's the only thing the storefront can switch on
	custom := urlrewrite.EntityType("wishlist_share")
	if got := routeKindFromEntity(custom); got != model.RouteKindPlugin {
		t.Errorf("routeKind = %s, want PLUGIN", got)
	}
	pk := pluginKindFor(custom)
	if pk == nil || *pk != "wishlist_share" {
		t.Errorf("pluginKind = %v, want the raw slug", pk)
	}
}

func TestRouteKindAndPluginKindAgree(t *testing.T) { // the invariant the storefront relies on: pluginKind is set exactly when the kind is PLUGIN
	for _, entity := range []urlrewrite.EntityType{
		urlrewrite.EntityTypeProduct, urlrewrite.EntityTypeCMSPage,
		"some_plugin_entity", "", "PRODUCT", // casing matters - the DB stores lowercase
	} {
		isPlugin := routeKindFromEntity(entity) == model.RouteKindPlugin
		hasPluginKind := pluginKindFor(entity) != nil
		if isPlugin != hasPluginKind {
			t.Errorf("%q: kind PLUGIN=%v but pluginKind set=%v, want them in lockstep", entity, isPlugin, hasPluginKind)
		}
	}
}
