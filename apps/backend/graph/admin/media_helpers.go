package admin

import (
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/medialibrary"
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
