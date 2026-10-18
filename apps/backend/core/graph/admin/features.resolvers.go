package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *queryResolver) ServerFeatures(ctx context.Context) (*model.ServerFeatures, error) {
	if _, err := auth.RequireActor(ctx); err != nil { // every signed-in caller reads this for nav gating, so it's authentication rather than a slug
		return nil, err
	}
	return &model.ServerFeatures{Signals: r.SignalsEnabled, Ai: r.AISvc.Enabled()}, nil
}
