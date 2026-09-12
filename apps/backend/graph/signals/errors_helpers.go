package signals

import (
	"fmt"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
)

func errInvalidID(field string, err error) error {
	return &apperr.Error{
		Code:    "INVALID_ID",
		Class:   apperr.ClassValidation,
		Message: fmt.Sprintf("%s is not a valid id", field),
		Cause:   err,
	}
}

func errNotFound(entity string) error {
	return apperr.New("NOT_FOUND", apperr.ClassNotFound, entity+" not found")
}

func errNotConfigured(subsystem string) error {
	return apperr.New("NOT_CONFIGURED", apperr.ClassUnavailable, subsystem+" is not configured")
}
