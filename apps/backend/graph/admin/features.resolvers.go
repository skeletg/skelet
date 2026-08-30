package admin

import (
	"context"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
)

func (r *queryResolver) ServerFeatures(ctx context.Context) (*model.ServerFeatures, error) {
	if _, err := auth.RequireAuth(ctx); err != nil { // every signed-in admin reads this for nav gating, so it's authentication rather than a slug
		return nil, err
	}
	return &model.ServerFeatures{B2b: r.B2BSvc.Enabled(), Signals: r.SignalsEnabled, Ai: r.AISvc.Enabled()}, nil
}
