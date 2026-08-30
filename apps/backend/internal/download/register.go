package download

import "github.com/skeletg/skelet/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrNotFound, "DOWNLOAD_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrExpired, "DOWNLOAD_EXPIRED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrLimitReached, "DOWNLOAD_LIMIT_REACHED", apperr.ClassConflict)
}
