// Package refs is how a plugin's own GraphQL types point back at core's records on the storefront API - only what a
// shopper may see there: products and the categories and collections they browse, visible on the request's channel.
// Each answers one record per id in the ids' order, nil for one not visible there
package refs

import (
	"context"
	"sync/atomic"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	adminrefs "github.com/skeletg/skelet/apps/backend/core/graph/admin/refs"
)

// The storefront graph's by-id queries, which it installs at boot
type Loaders struct {
	Products    adminrefs.Loader[model.Product]
	Categories  adminrefs.Loader[model.Category]
	Collections adminrefs.Loader[model.Collection]
}

var loaders atomic.Pointer[Loaders]

// Install is the storefront graph's to call, once, at boot
func Install(l Loaders) { loaders.Store(&l) }

func installed() Loaders {
	if l := loaders.Load(); l != nil {
		return *l
	}
	return Loaders{}
}

func Products(ctx context.Context, ids []string) ([]*model.Product, error) {
	return adminrefs.Align(ctx, ids, installed().Products, func(p *model.Product) string { return p.ID })
}

func Categories(ctx context.Context, ids []string) ([]*model.Category, error) {
	return adminrefs.Align(ctx, ids, installed().Categories, func(c *model.Category) string { return c.ID })
}

func Collections(ctx context.Context, ids []string) ([]*model.Collection, error) {
	return adminrefs.Align(ctx, ids, installed().Collections, func(c *model.Collection) string { return c.ID })
}
