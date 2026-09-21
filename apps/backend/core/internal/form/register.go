package form

import (
	"errors"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

var (
	ErrFormNotFound  = errors.New("form not found")
	ErrEmailRequired = errors.New("email is required")
)

func init() {
	apperr.RegisterSentinel(ErrFormNotFound, "SIGNUP_FORM_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrEmailRequired, "EMAIL_REQUIRED", apperr.ClassValidation)
}
