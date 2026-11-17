// Package refs is how a plugin's own GraphQL types point back at core's records on the storefront API - only what a
// shopper may see there: products and the categories and collections they browse, visible on the request's channel.
// Each answers one record per id in the ids' order, nil for one not visible there
package refs

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	adminrefs "github.com/skeletg/skelet/apps/backend/core/graph/admin/refs"
)

// The storefront graph's by-id queries, which it installs at boot
type Loaders struct {
	Products    func(ctx context.Context, ids []string) ([]*model.Product, error)
	Categories  func(ctx context.Context, ids []string) ([]*model.Category, error)
	Collections func(ctx context.Context, ids []string) ([]*model.Collection, error)
}

var loaders Loaders

func Install(l Loaders) { loaders = l }

func Products(ctx context.Context, ids []string) ([]*model.Product, error) {
	return adminrefs.Align(ctx, ids, loaders.Products, func(p *model.Product) string { return p.ID })
}

func Categories(ctx context.Context, ids []string) ([]*model.Category, error) {
	return adminrefs.Align(ctx, ids, loaders.Categories, func(c *model.Category) string { return c.ID })
}

func Collections(ctx context.Context, ids []string) ([]*model.Collection, error) {
	return adminrefs.Align(ctx, ids, loaders.Collections, func(c *model.Collection) string { return c.ID })
}
