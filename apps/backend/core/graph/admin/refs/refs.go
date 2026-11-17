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
	"strings"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Loader is one of the graph's by-id queries, with the most ids it takes in a call - its own cap, so an id past it is
// asked again rather than dropped
type Loader[T any] struct {
	Load  func(ctx context.Context, ids []string) ([]*T, error)
	Batch int
}

// The admin graph's by-id queries, which it installs at boot
type Loaders struct {
	Products    Loader[model.Product]
	Customers   Loader[model.Customer]
	Orders      Loader[model.Order]
	Categories  Loader[model.Category]
	Collections Loader[model.Collection]
}

var loaders atomic.Pointer[Loaders]

var ErrNotInstalled = errors.New("refs: the graph hasn't installed its loaders")

// Install is the admin graph's to call, once, at boot
func Install(l Loaders) { loaders.Store(&l) }

func installed() Loaders {
	if l := loaders.Load(); l != nil {
		return *l
	}
	return Loaders{}
}

func Products(ctx context.Context, ids []string) ([]*model.Product, error) {
	return Align(ctx, ids, installed().Products, func(p *model.Product) string { return p.ID })
}

func Customers(ctx context.Context, ids []string) ([]*model.Customer, error) {
	return Align(ctx, ids, installed().Customers, func(c *model.Customer) string { return c.ID })
}

func Orders(ctx context.Context, ids []string) ([]*model.Order, error) {
	return Align(ctx, ids, installed().Orders, func(o *model.Order) string { return o.ID })
}

func Categories(ctx context.Context, ids []string) ([]*model.Category, error) {
	return Align(ctx, ids, installed().Categories, func(c *model.Category) string { return c.ID })
}

func Collections(ctx context.Context, ids []string) ([]*model.Collection, error) {
	return Align(ctx, ids, installed().Collections, func(c *model.Collection) string { return c.ID })
}

// Align asks the loader for each distinct id once, its batch at a time, and answers in the ids' order with nil where
// nothing came back - shared by the storefront's refs. An id that isn't a uuid names nothing, so it's never asked
func Align[T any](ctx context.Context, ids []string, l Loader[T], id func(*T) string) ([]*T, error) {
	if l.Load == nil {
		return nil, ErrNotInstalled
	}
	batch := max(l.Batch, 1)
	canonical := make([]string, len(ids))
	seen := map[string]bool{}
	var distinct []string
	for i, v := range ids {
		u, err := uuid.Parse(v)
		if err != nil {
			continue
		}
		canonical[i] = u.String()
		if !seen[canonical[i]] {
			seen[canonical[i]] = true
			distinct = append(distinct, canonical[i])
		}
	}
	found := make(map[string]*T, len(distinct))
	for start := 0; start < len(distinct); start += batch {
		got, err := l.Load(ctx, distinct[start:min(start+batch, len(distinct))])
		if err != nil {
			return nil, err
		}
		for _, v := range got {
			if v != nil {
				found[strings.ToLower(id(v))] = v
			}
		}
	}
	out := make([]*T, len(ids))
	for i, c := range canonical {
		if c != "" {
			out[i] = found[c]
		}
	}
	return out, nil
}

// Each wraps a query that answers one record, for a record with no by-ids query. One that's gone, or that the caller
// may not see, is left out; anything else - the database, a deadline - fails the call, since a failure answered as
// "nothing there" would be cached as the truth
func Each[T any](one func(ctx context.Context, id string) (*T, error)) func(context.Context, []string) ([]*T, error) {
	return func(ctx context.Context, ids []string) ([]*T, error) {
		out := make([]*T, 0, len(ids))
		for _, id := range ids {
			v, err := one(ctx, id)
			switch apperr.ClassOf(err) {
			case apperr.ClassNotFound, apperr.ClassForbidden:
				continue
			}
			if err != nil {
				return nil, err
			}
			if v != nil {
				out = append(out, v)
			}
		}
		return out, nil
	}
}
