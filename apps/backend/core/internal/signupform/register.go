package signupform

import (
	"errors"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

var (
	ErrFormNotFound        = errors.New("form not found")
	ErrEmailRequired       = errors.New("email is required")
	ErrEmailInvalid        = errors.New("enter a valid email address")
	ErrNameTooLong         = errors.New("name is too long")
	ErrNameAndSlugRequired = errors.New("name and slug are required")
	ErrSlugTaken           = errors.New("a form with this slug already exists on this store")
	// Unknown, already used and expired all read the same, so the answer leaks nothing about the link
	ErrConfirmationInvalid = errors.New("this confirmation link is invalid or has expired")
)

func init() {
	apperr.RegisterSentinel(ErrFormNotFound, "SIGNUP_FORM_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrEmailRequired, "EMAIL_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrEmailInvalid, "SIGNUP_EMAIL_INVALID", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNameTooLong, "SIGNUP_NAME_TOO_LONG", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNameAndSlugRequired, "SIGNUP_FORM_NAME_SLUG_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrSlugTaken, "SIGNUP_FORM_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrConfirmationInvalid, "SIGNUP_CONFIRMATION_INVALID", apperr.ClassNotFound)
}
