package integration

import (
	"errors"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

var (
	ErrNotFound          = errors.New("integration not found")
	ErrUnknownPermission = errors.New("unknown permission")
	ErrNameRequired      = errors.New("integration name is required")
	ErrNameTooLong       = errors.New("integration name is too long")
	ErrExpiryInPast      = errors.New("expiry must be in the future")
)

func init() {
	apperr.RegisterSentinel(ErrNotFound, "INTEGRATION_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrUnknownPermission, "INTEGRATION_UNKNOWN_PERMISSION", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNameRequired, "INTEGRATION_NAME_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNameTooLong, "INTEGRATION_NAME_TOO_LONG", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrExpiryInPast, "INTEGRATION_EXPIRY_IN_PAST", apperr.ClassValidation)
}
