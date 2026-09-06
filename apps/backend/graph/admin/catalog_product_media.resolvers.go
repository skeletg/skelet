package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
)

// Media renders on every channel listing the product, so all media mutations carry the same all-listing-channels gate as the shared content rows
func (r *mutationResolver) RequestProductMediaUpload(ctx context.Context, input model.RequestProductMediaUploadInput) (*model.RequestMediaUploadPayload, error) {
	productID, err := parseID(input.ProductID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, productID); err != nil {
		return nil, err
	}
	domainInput := catalog.RequestMediaUploadInput{
		ProductID: productID,
		Filename:  input.Filename,
		MimeType:  input.MimeType,
		SizeBytes: int64(input.SizeBytes),
		AltText:   stringOr(input.AltText, ""),
		SortOrder: int32Or(input.SortOrder, 0),
	}
	if input.Target != nil {
		t, err := mediaUploadTargetFromGQL(*input.Target)
		if err != nil {
			return nil, err
		}
		domainInput.Target = &t
	}
	if input.Role != nil {
		r2, err := mediaRoleFromGQL(*input.Role)
		if err != nil {
			return nil, err
		}
		domainInput.Role = &r2
	}
	result, err := r.CatalogSvc.RequestMediaUpload(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return &model.RequestMediaUploadPayload{
		Media:     toProductMediaModel(result.Media),
		UploadURL: result.UploadURL,
		ExpiresAt: result.ExpiresAt,
	}, nil
}

func (r *mutationResolver) ConfirmProductMediaUpload(ctx context.Context, mediaID string, width int32, height int32) (*model.ProductMedia, error) {
	mid, err := parseID(mediaID, "media")
	if err != nil {
		return nil, err
	}
	// Load-then-gate: the media row is the only path back to the product whose listings scope this edit
	existing, err := r.CatalogSvc.GetProductMedia(ctx, mid)
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, existing.ProductID); err != nil {
		return nil, err
	}
	m, err := r.CatalogSvc.ConfirmMediaUpload(ctx, catalog.ConfirmMediaUploadInput{
		MediaID: mid,
		Width:   width,
		Height:  height,
	})
	if err != nil {
		return nil, err
	}
	return toProductMediaModel(m), nil
}

func (r *mutationResolver) UpdateProductMedia(ctx context.Context, input model.UpdateProductMediaInput) (*model.ProductMedia, error) {
	id, err := parseID(input.ID, "media")
	if err != nil {
		return nil, err
	}
	existing, err := r.CatalogSvc.GetProductMedia(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, existing.ProductID); err != nil {
		return nil, err
	}
	m, err := r.CatalogSvc.UpdateProductMedia(ctx, catalog.UpdateProductMediaInput{
		ID:        id,
		AltText:   input.AltText,
		SortOrder: input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return toProductMediaModel(m), nil
}

func (r *mutationResolver) DeleteProductMedia(ctx context.Context, id string, deleteFromStorage *bool) (bool, error) {
	mid, err := parseID(id, "media")
	if err != nil {
		return false, err
	}
	existing, err := r.CatalogSvc.GetProductMedia(ctx, mid)
	if err != nil {
		return false, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, existing.ProductID); err != nil {
		return false, err
	}
	del := deleteFromStorage == nil || *deleteFromStorage
	if err := r.CatalogSvc.DeleteProductMedia(ctx, mid, del); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) DeleteProductMediaBulk(ctx context.Context, productID string, ids []string, deleteFromStorage *bool) (int32, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return 0, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return 0, err
	}
	uids := make([]uuid.UUID, 0, len(ids))
	for _, raw := range ids {
		uid, err := parseID(raw, "media")
		if err != nil {
			return 0, err
		}
		uids = append(uids, uid)
	}
	del := deleteFromStorage == nil || *deleteFromStorage
	n, err := r.CatalogSvc.DeleteProductMediaBulk(ctx, pid, uids, del)
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

func (r *mutationResolver) ReorderProductMedia(ctx context.Context, productID string, mediaIds []string) ([]*model.ProductMedia, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(mediaIds))
	for _, s := range mediaIds {
		u, err := parseID(s, "media")
		if err != nil {
			return nil, err
		}
		ids = append(ids, u)
	}
	out, err := r.CatalogSvc.ReorderProductMedia(ctx, pid, ids)
	if err != nil {
		return nil, err
	}
	result := make([]*model.ProductMedia, len(out))
	for i, m := range out {
		result[i] = toProductMediaModel(m)
	}
	return result, nil
}

// SetProductMediaRole assigns a role (main, thumbnail, og_image) to a media item.
func (r *mutationResolver) SetProductMediaRole(ctx context.Context, productID string, role model.ProductMediaRole, mediaID string) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	mid, err := parseID(mediaID, "media")
	if err != nil {
		return nil, err
	}
	domainRole, err := mediaRoleFromGQL(role)
	if err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.SetProductMediaRole(ctx, pid, mid, domainRole)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

// ClearProductMediaRole removes a role assignment from a product.
func (r *mutationResolver) ClearProductMediaRole(ctx context.Context, productID string, role model.ProductMediaRole) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	domainRole, err := mediaRoleFromGQL(role)
	if err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.ClearProductMediaRole(ctx, pid, domainRole)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}
