package admin

import (
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

func mediaRoleFromGQL(r model.ProductMediaRole) (catalog.MediaRole, error) {
	switch r {
	case model.ProductMediaRoleMain:
		return catalog.MediaRoleMain, nil
	case model.ProductMediaRoleThumbnail:
		return catalog.MediaRoleThumbnail, nil
	case model.ProductMediaRoleOgImage:
		return catalog.MediaRoleOGImage, nil
	}
	return "", fmt.Errorf("unknown media role: %s", r)
}

func mediaUploadTargetFromGQL(t model.ProductMediaUploadTarget) (catalog.MediaUploadTarget, error) {
	switch t {
	case model.ProductMediaUploadTargetGallery:
		return catalog.MediaUploadTargetGallery, nil
	case model.ProductMediaUploadTargetDescription:
		return catalog.MediaUploadTargetDescription, nil
	case model.ProductMediaUploadTargetShortDescription:
		return catalog.MediaUploadTargetShortDescription, nil
	}
	return "", fmt.Errorf("unknown media upload target: %s", t)
}
