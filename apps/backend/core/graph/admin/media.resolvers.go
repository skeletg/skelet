package admin

import (
	"context"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mediaAssetResolver) ThumbnailURL(ctx context.Context, obj *model.MediaAsset, width int32) (string, error) {
	if obj.ObjectKey == "" {
		return obj.URL, nil
	}
	return r.MediaLibrarySvc.ThumbnailURL(ctx, thumbnailSource(obj), int(width)), nil
}

func (r *mutationResolver) SetMediaFocalPoint(ctx context.Context, assetID string, x *float64, y *float64) (*model.MediaAsset, error) {
	if err := r.requireGlobal(ctx, "media.manage"); err != nil {
		return nil, err
	}
	current, err := r.libraryAsset(ctx, assetID, medialibrary.PermPrivateManage)
	if err != nil {
		return nil, err
	}
	asset, err := r.MediaLibrarySvc.SetFocalPoint(ctx, current.ID, current.Visibility, x, y)
	if err != nil {
		return nil, err
	}
	return toMediaAssetModel(asset), nil
}

func (r *mutationResolver) CreateMediaDirectory(ctx context.Context, path string, name string, visibility *model.MediaVisibility) (*model.MediaDirectoryChild, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return nil, err
	}
	in := mediaVisibilityFromGQL(visibility)
	if err := r.requirePrivate(ctx, medialibrary.PermPrivateManage, in); err != nil {
		return nil, err
	}
	dir, err := r.MediaLibrarySvc.CreateDirectory(ctx, path, name, in)
	if err != nil {
		return nil, err
	}
	return toMediaDirectoryChildModel(dir), nil
}

func (r *mutationResolver) RequestMediaAssetUpload(ctx context.Context, input model.RequestMediaAssetUploadInput) (*model.RequestMediaAssetUploadPayload, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return nil, err
	}
	in := mediaVisibilityFromGQL(input.Visibility)
	if err := r.requirePrivate(ctx, medialibrary.PermPrivateManage, in); err != nil {
		return nil, err
	}
	result, err := r.MediaLibrarySvc.RequestUpload(ctx, medialibrary.RequestUploadInput{
		Path:       input.Path,
		Visibility: in,
		Filename:   input.Filename,
		MimeType:   input.MimeType,
		SizeBytes:  int64(input.SizeBytes),
		Title:      stringOr(input.Title, ""),
		AltText:    stringOr(input.AltText, ""),
		Caption:    stringOr(input.Caption, ""),
		Notes:      stringOr(input.Notes, ""),
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
	pending, err := r.libraryAsset(ctx, assetID, medialibrary.PermPrivateManage)
	if err != nil {
		return nil, err
	}
	asset, err := r.MediaLibrarySvc.ConfirmUpload(ctx, medialibrary.ConfirmUploadInput{
		AssetID:    pending.ID,
		Visibility: pending.Visibility,
		Width:      width,
		Height:     height,
	})
	if err != nil {
		return nil, err
	}
	return toMediaAssetModel(asset), nil
}

func (r *mutationResolver) UpdateMediaAsset(ctx context.Context, input model.UpdateMediaAssetInput) (*model.MediaAsset, error) {
	if err := r.requireGlobal(ctx, "media.manage"); err != nil {
		return nil, err
	}
	current, err := r.libraryAsset(ctx, input.ID, medialibrary.PermPrivateManage)
	if err != nil {
		return nil, err
	}
	asset, err := r.MediaLibrarySvc.UpdateAsset(ctx, medialibrary.UpdateAssetInput{
		ID:         current.ID,
		Visibility: current.Visibility,
		Title:      input.Title,
		AltText:    input.AltText,
		Caption:    input.Caption,
		Notes:      input.Notes,
	})
	if err != nil {
		return nil, err
	}
	return toMediaAssetModel(asset), nil
}

func (r *mutationResolver) MoveMediaAsset(ctx context.Context, id string, targetPath string, filename *string, visibility *model.MediaVisibility) (*model.MediaAsset, error) {
	if err := r.requireGlobal(ctx, "media.manage"); err != nil {
		return nil, err
	}
	current, err := r.libraryAsset(ctx, id, medialibrary.PermPrivateManage)
	if err != nil {
		return nil, err
	}
	move := medialibrary.MoveAssetInput{ID: current.ID, From: current.Visibility, TargetPath: targetPath, Filename: filename}
	if visibility != nil {
		to := mediaVisibilityFromGQL(visibility)
		// Crossing takes the grant for both halves: into the private one, or out of it to an address anyone can open
		if err := r.requirePrivate(ctx, medialibrary.PermPrivateManage, to); err != nil {
			return nil, err
		}
		move.Visibility = &to
	}
	asset, err := r.MediaLibrarySvc.MoveAsset(ctx, move)
	if err != nil {
		return nil, err
	}
	return toMediaAssetModel(asset), nil
}

func (r *mutationResolver) DeleteMediaAsset(ctx context.Context, id string, deleteFromStorage *bool) (bool, error) {
	if err := r.requireGlobal(ctx, "media.manage"); err != nil {
		return false, err
	}
	current, err := r.libraryAsset(ctx, id, medialibrary.PermPrivateManage)
	if err != nil {
		return false, err
	}
	if err := r.MediaLibrarySvc.DeleteAsset(ctx, current.ID, current.Visibility, boolOrDefault(deleteFromStorage, true)); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) DeleteMediaDirectory(ctx context.Context, path string, recursive bool, visibility *model.MediaVisibility) (bool, error) {
	if err := r.requireGlobal(ctx, "media.manage"); err != nil {
		return false, err
	}
	in := mediaVisibilityFromGQL(visibility)
	if err := r.requirePrivate(ctx, medialibrary.PermPrivateManage, in); err != nil {
		return false, err
	}
	if err := r.MediaLibrarySvc.DeleteDirectory(ctx, path, recursive, in); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) RenameMediaDirectory(ctx context.Context, path string, newName string, visibility *model.MediaVisibility) (*model.MediaDirectoryChild, error) {
	if err := r.requireGlobal(ctx, "media.manage"); err != nil {
		return nil, err
	}
	in := mediaVisibilityFromGQL(visibility)
	if err := r.requirePrivate(ctx, medialibrary.PermPrivateManage, in); err != nil {
		return nil, err
	}
	dir, err := r.MediaLibrarySvc.RenameDirectory(ctx, path, newName, in)
	if err != nil {
		return nil, err
	}
	return toMediaDirectoryChildModel(dir), nil
}

func (r *mutationResolver) ShareMediaAsset(ctx context.Context, input model.ShareMediaAssetInput) (*model.MediaShare, error) {
	if err := r.requireSharing(ctx); err != nil {
		return nil, err
	}
	asset, err := r.libraryAsset(ctx, input.AssetID, medialibrary.PermPrivateView)
	if err != nil {
		return nil, err
	}
	if asset.Status != medialibrary.AssetStatusReady {
		return nil, medialibrary.ErrUploadNotReceived
	}
	if asset.Visibility != medialibrary.Private {
		return nil, medialibrary.ErrSharePublic
	}
	customerID, err := parseID(input.CustomerID, "customer")
	if err != nil {
		return nil, err
	}
	buyer, err := r.CustomerSvc.GetByID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "customers.view", buyer.ChannelID, nil); err != nil {
		return nil, err
	}
	if !buyer.IsActive {
		return nil, download.ErrInactiveCustomer
	}
	title := asset.Title
	if title == "" {
		title = asset.Filename
	}
	grant, err := r.DownloadSvc.ShareAsset(ctx, download.ShareInput{
		AssetID: asset.ID, CustomerID: customerID, Email: buyer.Email, Title: title,
		MaxDownloads: input.MaxDownloads, ExpiresAt: input.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	return toMediaShareModel(grant), nil
}

func (r *mutationResolver) RevokeMediaShare(ctx context.Context, id string) (bool, error) {
	if err := r.requireSharing(ctx); err != nil {
		return false, err
	}
	shareID, err := parseID(id, "share")
	if err != nil {
		return false, err
	}
	if err := r.DownloadSvc.Unshare(ctx, shareID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) MediaDirectory(ctx context.Context, path string, visibility *model.MediaVisibility) (*model.MediaDirectory, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return nil, err
	}
	in := mediaVisibilityFromGQL(visibility)
	if err := r.requirePrivate(ctx, medialibrary.PermPrivateView, in); err != nil {
		return nil, err
	}
	dir, err := r.MediaLibrarySvc.GetDirectory(ctx, path, in)
	if err != nil {
		return nil, err
	}
	return toMediaDirectoryModel(dir), nil
}

func (r *queryResolver) MediaAsset(ctx context.Context, id string) (*model.MediaAsset, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return nil, err
	}
	asset, err := r.libraryAsset(ctx, id, medialibrary.PermPrivateView)
	if err != nil {
		return nil, err
	}
	return toMediaAssetModel(asset), nil
}

func (r *queryResolver) SearchMediaAssets(ctx context.Context, query *string, path *string, mimeGroup *model.MediaMimeGroup, sort *model.MediaAssetSort, limit *int32, offset *int32, visibility *model.MediaVisibility) (*model.MediaAssetSearchResult, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return nil, err
	}
	in := mediaVisibilityFromGQL(visibility)
	if err := r.requirePrivate(ctx, medialibrary.PermPrivateView, in); err != nil {
		return nil, err
	}
	var group *medialibrary.MimeGroup
	if mimeGroup != nil {
		g := mediaMimeGroupFromGQL(*mimeGroup)
		group = &g
	}
	var order string
	if sort != nil {
		order = strings.ToLower(string(*sort))
	}
	result, err := r.MediaLibrarySvc.SearchAssets(ctx, medialibrary.SearchInput{
		Query:      query,
		Path:       path,
		Visibility: in,
		MimeGroup:  group,
		Sort:       order,
		Limit:      int32Or(limit, 50),
		Offset:     int32Or(offset, 0),
	})
	if err != nil {
		return nil, err
	}
	return toMediaAssetSearchResultModel(result), nil
}

func (r *queryResolver) AllMediaDirectories(ctx context.Context, visibility *model.MediaVisibility) ([]*model.MediaDirectoryChild, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return nil, err
	}
	in := mediaVisibilityFromGQL(visibility)
	if err := r.requirePrivate(ctx, medialibrary.PermPrivateView, in); err != nil {
		return nil, err
	}
	dirs, err := r.MediaLibrarySvc.ListAllDirectories(ctx, in)
	if err != nil {
		return nil, err
	}
	result := make([]*model.MediaDirectoryChild, len(dirs))
	for i, d := range dirs {
		result[i] = toMediaDirectoryChildModel(d)
	}
	return result, nil
}

func (r *queryResolver) MediaPrivateArea(ctx context.Context) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return false, err
	}
	return r.MediaLibrarySvc.HasPrivate(), nil
}

func (r *queryResolver) MediaAssetShares(ctx context.Context, assetID string) ([]*model.MediaShare, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return nil, err
	}
	asset, err := r.libraryAsset(ctx, assetID, medialibrary.PermPrivateView)
	if err != nil {
		return nil, err
	}
	grants, err := r.DownloadSvc.SharesOf(ctx, asset.ID)
	if err != nil {
		return nil, err
	}
	return r.sharesFor(ctx, grants)
}

func (r *queryResolver) MediaUploadTypes(ctx context.Context) ([]string, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return nil, err
	}
	return medialibrary.UploadTypes(), nil
}

func (r *Resolver) MediaAsset() MediaAssetResolver { return &mediaAssetResolver{r} }

type mediaAssetResolver struct{ *Resolver }
