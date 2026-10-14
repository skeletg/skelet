package front

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
)

func (r *queryResolver) PublicStores(ctx context.Context, input *model.PublicStoresInput) (*model.PublicStoresResult, error) {
	return r.publicStores(ctx, input)
}
