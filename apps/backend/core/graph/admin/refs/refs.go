// Package refs is how a plugin's own GraphQL types point back at core's records on the admin API - a warranty's
// product, a note's customer - answered with core's own models through core's own gated queries
//
//	func (adminResolver) AcmeWarrantyProduct(ctx context.Context, objs []*model.AcmeWarranty) ([]*model.Product, error) {
//	    if err := sdk.RequirePermission(ctx, "catalog.view"); err != nil {
//	        return nil, err
//	    }
//	    return refs.Products(ctx, sdk.IDs(objs, func(w *model.AcmeWarranty) string { return w.ProductID }))
//	}
//
// Each answers one record per id in the ids' order, nil for an id that names nothing the caller may see
package refs

import (
	"context"
	"errors"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
)

// The admin graph's by-id queries, which it installs at boot
type Loaders struct {
	Products    func(ctx context.Context, ids []string) ([]*model.Product, error)
	Customers   func(ctx context.Context, ids []string) ([]*model.Customer, error)
	Orders      func(ctx context.Context, ids []string) ([]*model.Order, error)
	Categories  func(ctx context.Context, ids []string) ([]*model.Category, error)
	Collections func(ctx context.Context, ids []string) ([]*model.Collection, error)
}

var loaders Loaders

var ErrNotInstalled = errors.New("refs: the admin graph hasn't installed its loaders")

func Install(l Loaders) { loaders = l }

func Products(ctx context.Context, ids []string) ([]*model.Product, error) {
	return Align(ctx, ids, loaders.Products, func(p *model.Product) string { return p.ID })
}

func Customers(ctx context.Context, ids []string) ([]*model.Customer, error) {
	return Align(ctx, ids, loaders.Customers, func(c *model.Customer) string { return c.ID })
}

func Orders(ctx context.Context, ids []string) ([]*model.Order, error) {
	return Align(ctx, ids, loaders.Orders, func(o *model.Order) string { return o.ID })
}

func Categories(ctx context.Context, ids []string) ([]*model.Category, error) {
	return Align(ctx, ids, loaders.Categories, func(c *model.Category) string { return c.ID })
}

func Collections(ctx context.Context, ids []string) ([]*model.Collection, error) {
	return Align(ctx, ids, loaders.Collections, func(c *model.Collection) string { return c.ID })
}

// The most ids one loader call takes - core's by-id queries cap there
const chunk = 100

// Align asks load for each distinct id once, a hundred at a time, and answers in the ids' order with nil where nothing
// came back - shared by the storefront's refs
func Align[T any](ctx context.Context, ids []string, load func(context.Context, []string) ([]*T, error), id func(*T) string) ([]*T, error) {
	if load == nil {
		return nil, ErrNotInstalled
	}
	seen := map[string]bool{}
	var distinct []string
	for _, v := range ids {
		if v != "" && !seen[v] {
			seen[v] = true
			distinct = append(distinct, v)
		}
	}
	found := make(map[string]*T, len(distinct))
	for start := 0; start < len(distinct); start += chunk {
		got, err := load(ctx, distinct[start:min(start+chunk, len(distinct))])
		if err != nil {
			return nil, err
		}
		for _, v := range got {
			if v != nil {
				found[id(v)] = v
			}
		}
	}
	out := make([]*T, len(ids))
	for i, v := range ids {
		out[i] = found[v]
	}
	return out, nil
}
