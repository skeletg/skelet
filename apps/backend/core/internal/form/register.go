package form

import (
	"errors"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

var (
	ErrFormNotFound  = errors.New("form not found")
	ErrEmailRequired = errors.New("email is required")
	// Unknown, already used and expired all read the same, so the answer leaks nothing about the link
	ErrConfirmationInvalid = errors.New("this confirmation link is invalid or has expired")
)

func init() {
	apperr.RegisterSentinel(ErrFormNotFound, "SIGNUP_FORM_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrEmailRequired, "EMAIL_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrConfirmationInvalid, "SIGNUP_CONFIRMATION_INVALID", apperr.ClassNotFound)
}
