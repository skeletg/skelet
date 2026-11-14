package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func toMediaDirectoryModel(d medialibrary.Directory) *model.MediaDirectory {
	dirs := make([]*model.MediaDirectoryChild, len(d.Directories))
	for i, child := range d.Directories {
		dirs[i] = toMediaDirectoryChildModel(child)
	}
	assets := make([]*model.MediaAsset, len(d.Assets))
	for i, asset := range d.Assets {
		assets[i] = toMediaAssetModel(asset)
	}
	untracked := make([]*model.UntrackedMediaFile, len(d.UntrackedFiles))
	for i, f := range d.UntrackedFiles {
		untracked[i] = toUntrackedMediaFileModel(f)
	}
	return &model.MediaDirectory{
		Path:           d.Path,
		Name:           d.Name,
		Directories:    dirs,
		Assets:         assets,
		UntrackedFiles: untracked,
	}
}

func toUntrackedMediaFileModel(f medialibrary.UntrackedFile) *model.UntrackedMediaFile {
	return &model.UntrackedMediaFile{
		ObjectKey:    f.ObjectKey,
		URL:          f.URL,
		Filename:     f.Filename,
		MimeType:     f.MimeType,
		SizeBytes:    int32(f.SizeBytes),
		LastModified: f.LastModified,
	}
}

func toMediaDirectoryChildModel(d medialibrary.DirectoryChild) *model.MediaDirectoryChild {
	return &model.MediaDirectoryChild{
		Path: d.Path,
		Name: d.Name,
	}
}

func toMediaAssetSearchResultModel(result medialibrary.SearchResult) *model.MediaAssetSearchResult {
	items := make([]*model.MediaAsset, len(result.Items))
	for i, asset := range result.Items {
		items[i] = toMediaAssetModel(asset)
	}
	return &model.MediaAssetSearchResult{
		Items:      items,
		TotalCount: result.TotalCount,
	}
}

func toMediaAssetModel(asset medialibrary.Asset) *model.MediaAsset {
	return &model.MediaAsset{
		ID:            asset.ID.String(),
		Path:          asset.Path(),
		DirectoryPath: asset.DirectoryPrefix,
		Filename:      asset.Filename,
		URL:           asset.URL,
		Visibility:    toMediaVisibilityModel(asset.Visibility),
		ObjectKey:     asset.ObjectKey,
		FocalX:        asset.FocalX,
		FocalY:        asset.FocalY,
		MimeType:      asset.MimeType,
		MimeGroup:     toMediaMimeGroupModel(asset.MimeGroup()),
		SizeBytes:     int32(asset.SizeBytes),
		Width:         asset.Width,
		Height:        asset.Height,
		Status:        toMediaAssetStatusModel(asset.Status),
		Title:         asset.Title,
		AltText:       asset.AltText,
		Caption:       asset.Caption,
		Notes:         asset.Notes,
		CreatedAt:     asset.CreatedAt,
		UpdatedAt:     asset.UpdatedAt,
	}
}

func thumbnailSource(a *model.MediaAsset) medialibrary.Asset {
	status := medialibrary.AssetStatusPending
	if a.Status == model.MediaAssetStatusReady {
		status = medialibrary.AssetStatusReady
	}
	return medialibrary.Asset{ObjectKey: a.ObjectKey, MimeType: a.MimeType, Width: a.Width, Status: status, Visibility: mediaVisibilityFromGQL(&a.Visibility)}
}

func toMediaVisibilityModel(v medialibrary.Visibility) model.MediaVisibility {
	if v == medialibrary.Private {
		return model.MediaVisibilityPrivate
	}
	return model.MediaVisibilityPublic
}

func mediaVisibilityFromGQL(v *model.MediaVisibility) medialibrary.Visibility {
	if v != nil && *v == model.MediaVisibilityPrivate {
		return medialibrary.Private
	}
	return medialibrary.Public
}

func toMediaShareModel(g download.Grant) *model.MediaShare {
	share := &model.MediaShare{
		ID: g.ID.String(), Email: g.Email, DownloadsUsed: g.DownloadsUsed,
		MaxDownloads: g.MaxDownloads, ExpiresAt: g.ExpiresAt, CreatedAt: g.CreatedAt,
	}
	if g.CustomerID != nil {
		id := g.CustomerID.String()
		share.CustomerID = &id
	}
	return share
}

func (r *Resolver) requirePrivate(ctx context.Context, slug string, halves ...medialibrary.Visibility) error {
	return medialibrary.RequireHalf(ctx, r.AdminSvc, slug, halves...)
}

// The only by-id read of a library file in this package, so a private one never reaches a caller without slug
func (r *Resolver) libraryAsset(ctx context.Context, rawID, slug string) (medialibrary.Asset, error) {
	id, err := parseID(rawID, "media asset")
	if err != nil {
		return medialibrary.Asset{}, err
	}
	asset, err := r.MediaLibrarySvc.GetAnyAsset(ctx, id)
	if err != nil {
		return medialibrary.Asset{}, err
	}
	if err := r.requirePrivate(ctx, slug, asset.Visibility); err != nil {
		return medialibrary.Asset{}, err
	}
	return asset, nil
}

func (r *Resolver) requireSharing(ctx context.Context) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.view"); err != nil {
		return err
	}
	return r.requireGlobal(ctx, medialibrary.PermShare)
}

// Who a file is shared with is customer data: the address is kept back for a customer on a channel the caller can't see customers of
func (r *Resolver) sharesFor(ctx context.Context, grants []download.Grant) ([]*model.MediaShare, error) {
	seen, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "customers.view")
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(grants))
	for _, g := range grants {
		if g.CustomerID != nil {
			ids = append(ids, *g.CustomerID)
		}
	}
	buyers, err := r.CustomerSvc.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*model.MediaShare, len(grants))
	for i, g := range grants {
		out[i] = toMediaShareModel(g)
		if seen.Bypass() {
			continue
		}
		var buyer customer.Customer
		known := false
		if g.CustomerID != nil {
			buyer, known = buyers[*g.CustomerID]
		}
		if !known || !seen.Allows(buyer.ChannelID) {
			out[i].Email, out[i].CustomerID = "", nil
		}
	}
	return out, nil
}

func toMediaAssetStatusModel(status medialibrary.AssetStatus) model.MediaAssetStatus {
	switch status {
	case medialibrary.AssetStatusReady:
		return model.MediaAssetStatusReady
	default:
		return model.MediaAssetStatusPending
	}
}

func toMediaMimeGroupModel(group medialibrary.MimeGroup) model.MediaMimeGroup {
	switch group {
	case medialibrary.MimeGroupImage:
		return model.MediaMimeGroupImage
	default:
		return model.MediaMimeGroupDocument
	}
}

func mediaMimeGroupFromGQL(group model.MediaMimeGroup) medialibrary.MimeGroup {
	switch group {
	case model.MediaMimeGroupImage:
		return medialibrary.MimeGroupImage
	default:
		return medialibrary.MimeGroupDocument
	}
}
