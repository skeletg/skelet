package admin

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminsearch"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// What ⌘K may search for this admin - each kind under the same permission and channel scope as its list page, so the
// palette never shows a row the list would hide. A kind the admin can't read, or can't read in the header's channel,
// is left out rather than failing the whole search
func (r *queryResolver) adminSearchScope(ctx context.Context) (adminsearch.Scope, error) {
	var scope adminsearch.Scope
	var err error
	if scope.Orders, scope.OrderChannels, err = r.searchableKind(ctx, "orders.view"); err != nil {
		return adminsearch.Scope{}, err
	}
	if scope.Customers, scope.CustomerChannels, err = r.searchableKind(ctx, "customers.view"); err != nil {
		return adminsearch.Scope{}, err
	}
	// Catalog rows are global, as on the products list
	scope.Products = auth.RequirePermission(ctx, r.AdminSvc, "catalog.view") == nil
	return scope, nil
}

func (r *queryResolver) searchableKind(ctx context.Context, slug string) (bool, []uuid.UUID, error) {
	if auth.RequirePermission(ctx, r.AdminSvc, slug) != nil {
		return false, nil, nil
	}
	channels, none, err := r.readableChannels(ctx, slug, nil)
	if errors.Is(err, auth.ErrForbidden) || none {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return true, channels, nil
}

func toAdminSearchResult(res adminsearch.Result) *model.AdminSearchResult {
	out := &model.AdminSearchResult{
		Orders:    make([]*model.AdminSearchOrder, len(res.Orders)),
		Customers: make([]*model.AdminSearchCustomer, len(res.Customers)),
		Products:  make([]*model.AdminSearchProduct, len(res.Products)),
	}
	for i, o := range res.Orders {
		out.Orders[i] = &model.AdminSearchOrder{ID: o.ID.String(), Number: o.Number, Email: o.Email, Name: o.Name}
	}
	for i, c := range res.Customers {
		out.Customers[i] = &model.AdminSearchCustomer{ID: c.ID.String(), Email: c.Email, Name: c.Name}
	}
	for i, p := range res.Products {
		out.Products[i] = &model.AdminSearchProduct{ID: p.ID.String(), Sku: p.SKU, Name: p.Name}
	}
	return out
}
