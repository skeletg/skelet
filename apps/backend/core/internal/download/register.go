package download

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrNotFound, "DOWNLOAD_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrExpired, "DOWNLOAD_EXPIRED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrLimitReached, "DOWNLOAD_LIMIT_REACHED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrInvalidShare, "DOWNLOAD_INVALID_SHARE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInactiveCustomer, "DOWNLOAD_INACTIVE_CUSTOMER", apperr.ClassValidation)
}
