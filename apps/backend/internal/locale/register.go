package locale

import "github.com/skeletg/skelet/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrLocaleNotFound, "LOCALE_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrLocaleCodeTaken, "LOCALE_CODE_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrLocaleInUse, "LOCALE_IN_USE", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidCode, "LOCALE_INVALID_CODE", apperr.ClassValidation)
}
