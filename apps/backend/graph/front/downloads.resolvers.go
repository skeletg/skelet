package front

import (
	"context"

	"github.com/skeletg/skelet/graph/front/model"
	"github.com/skeletg/skelet/internal/auth"
)

// downloadUrl is the token-gated link - hitting it consumes one use and redirects to the file
func (r *queryResolver) MyDownloads(ctx context.Context) ([]*model.DownloadGrant, error) {
	claims, err := auth.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	grants, err := r.DownloadSvc.ListForCustomer(ctx, claims.CustomerID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.DownloadGrant, len(grants))
	for i, g := range grants {
		out[i] = &model.DownloadGrant{
			ID:            g.ID.String(),
			Title:         g.Title,
			ProductID:     g.ProductID.String(),
			DownloadURL:   "/download/" + g.Token,
			DownloadsUsed: g.DownloadsUsed,
			MaxDownloads:  g.MaxDownloads,
			ExpiresAt:     g.ExpiresAt,
			CreatedAt:     g.CreatedAt,
		}
	}
	return out, nil
}
