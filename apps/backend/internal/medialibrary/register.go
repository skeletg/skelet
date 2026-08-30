package medialibrary

import "github.com/skeletg/skelet/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrAssetNotFound, "MEDIA_ASSET_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrPathTaken, "MEDIA_PATH_TAKEN", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrAssetNotPending, "MEDIA_ASSET_NOT_PENDING", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrDirectoryNotEmpty, "MEDIA_DIRECTORY_NOT_EMPTY", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidMimeType, "MEDIA_INVALID_MIME_TYPE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrFileTooLarge, "MEDIA_FILE_TOO_LARGE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrUploadNotReceived, "MEDIA_UPLOAD_NOT_RECEIVED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrInvalidPath, "MEDIA_INVALID_PATH", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidFilename, "MEDIA_INVALID_FILENAME", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrRootRenameNotAllow, "MEDIA_ROOT_RENAME_NOT_ALLOWED", apperr.ClassValidation)
}
