package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/medialibrary"
)

func (r *mutationResolver) SetMediaFocalPoint(ctx context.Context, assetID string, x *float64, y *float64) (*model.MediaAsset, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(assetID, "asset")
	if err != nil {
		return nil, err
	}
	asset, err := r.MediaLibrarySvc.SetFocalPoint(ctx, id, x, y)
	if err != nil {
		return nil, err
	}
	return toMediaAssetModel(asset), nil
}

func (r *mutationResolver) CreateMediaDirectory(ctx context.Context, path string, name string) (*model.MediaDirectoryChild, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return nil, err
	}
	dir, err := r.MediaLibrarySvc.CreateDirectory(ctx, path, name)
	if err != nil {
		return nil, err
	}
	return toMediaDirectoryChildModel(dir), nil
}

func (r *mutationResolver) RequestMediaAssetUpload(ctx context.Context, input model.RequestMediaAssetUploadInput) (*model.RequestMediaAssetUploadPayload, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return nil, err
	}
	result, err := r.MediaLibrarySvc.RequestUpload(ctx, medialibrary.RequestUploadInput{
		Path:      input.Path,
		Filename:  input.Filename,
		MimeType:  input.MimeType,
		SizeBytes: int64(input.SizeBytes),
		Title:     stringOr(input.Title, ""),
		AltText:   stringOr(input.AltText, ""),
		Caption:   stringOr(input.Caption, ""),
		Notes:     stringOr(input.Notes, ""),
	})
	if err != nil {
		return nil, err
	}
	return &model.RequestMediaAssetUploadPayload{
		Asset:     toMediaAssetModel(result.Asset),
		UploadURL: result.UploadURL,
		ExpiresAt: result.ExpiresAt,
	}, nil
}

func (r *mutationResolver) ConfirmMediaAssetUpload(ctx context.Context, assetID string, width *int32, height *int32) (*model.MediaAsset, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(assetID, "media asset")
	if err != nil {
		return nil, err
	}
	asset, err := r.MediaLibrarySvc.ConfirmUpload(ctx, medialibrary.ConfirmUploadInput{
		AssetID: id,
		Width:   width,
		Height:  height,
	})
	if err != nil {
		return nil, err
	}
	return toMediaAssetModel(asset), nil
}

func (r *mutationResolver) UpdateMediaAsset(ctx context.Context, input model.UpdateMediaAssetInput) (*model.MediaAsset, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(input.ID, "media asset")
	if err != nil {
		return nil, err
	}
	asset, err := r.MediaLibrarySvc.UpdateAsset(ctx, medialibrary.UpdateAssetInput{
		ID:      id,
		Title:   input.Title,
		AltText: input.AltText,
		Caption: input.Caption,
		Notes:   input.Notes,
	})
	if err != nil {
		return nil, err
	}
	return toMediaAssetModel(asset), nil
}

func (r *mutationResolver) MoveMediaAsset(ctx context.Context, id string, targetPath string, filename *string) (*model.MediaAsset, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return nil, err
	}
	assetID, err := parseID(id, "media asset")
	if err != nil {
		return nil, err
	}
	asset, err := r.MediaLibrarySvc.MoveAsset(ctx, medialibrary.MoveAssetInput{
		ID:         assetID,
		TargetPath: targetPath,
		Filename:   filename,
	})
	if err != nil {
		return nil, err
	}
	return toMediaAssetModel(asset), nil
}

func (r *mutationResolver) DeleteMediaAsset(ctx context.Context, id string, deleteFromStorage *bool) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return false, err
	}
	assetID, err := parseID(id, "media asset")
	if err != nil {
		return false, err
	}
	if err := r.MediaLibrarySvc.DeleteAsset(ctx, assetID, boolOrDefault(deleteFromStorage, true)); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) DeleteMediaDirectory(ctx context.Context, path string, recursive bool) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return false, err
	}
	if err := r.MediaLibrarySvc.DeleteDirectory(ctx, path, recursive); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) RenameMediaDirectory(ctx context.Context, path string, newName string) (*model.MediaDirectoryChild, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return nil, err
	}
	dir, err := r.MediaLibrarySvc.RenameDirectory(ctx, path, newName)
	if err != nil {
		return nil, err
	}
	return toMediaDirectoryChildModel(dir), nil
}

func (r *queryResolver) MediaDirectory(ctx context.Context, path string) (*model.MediaDirectory, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return nil, err
	}
	dir, err := r.MediaLibrarySvc.GetDirectory(ctx, path)
	if err != nil {
		return nil, err
	}
	return toMediaDirectoryModel(dir), nil
}

func (r *queryResolver) MediaAsset(ctx context.Context, id string) (*model.MediaAsset, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return nil, err
	}
	assetID, err := parseID(id, "media asset")
	if err != nil {
		return nil, err
	}
	asset, err := r.MediaLibrarySvc.GetAsset(ctx, assetID)
	if err != nil {
		return nil, err
	}
	return toMediaAssetModel(asset), nil
}

func (r *queryResolver) SearchMediaAssets(ctx context.Context, query *string, path *string, mimeGroup *model.MediaMimeGroup, limit *int32, offset *int32) (*model.MediaAssetSearchResult, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return nil, err
	}
	var group *medialibrary.MimeGroup
	if mimeGroup != nil {
		g := mediaMimeGroupFromGQL(*mimeGroup)
		group = &g
	}
	result, err := r.MediaLibrarySvc.SearchAssets(ctx, medialibrary.SearchInput{
		Query:     query,
		Path:      path,
		MimeGroup: group,
		Limit:     int32Or(limit, 50),
		Offset:    int32Or(offset, 0),
	})
	if err != nil {
		return nil, err
	}
	return toMediaAssetSearchResultModel(result), nil
}

func (r *queryResolver) AllMediaDirectories(ctx context.Context) ([]*model.MediaDirectoryChild, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return nil, err
	}
	dirs, err := r.MediaLibrarySvc.ListAllDirectories(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*model.MediaDirectoryChild, len(dirs))
	for i, d := range dirs {
		result[i] = toMediaDirectoryChildModel(d)
	}
	return result, nil
}
