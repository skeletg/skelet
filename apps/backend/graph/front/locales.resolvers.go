package front

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
)

func (r *queryResolver) Locales(ctx context.Context, activeOnly *bool) ([]*model.Locale, error) {
	rows, err := r.LocaleSvc.List(ctx, boolOrFalse(activeOnly))
	if err != nil {
		return nil, err
	}
	out := make([]*model.Locale, len(rows))
	for i, l := range rows {
		out[i] = toLocaleModel(l)
	}
	return out, nil
}

func (r *queryResolver) Locale(ctx context.Context, code string) (*model.Locale, error) {
	l, err := r.LocaleSvc.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return toLocaleModel(l), nil
}
