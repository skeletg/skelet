package locale

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrLocaleNotFound, "LOCALE_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrLocaleCodeTaken, "LOCALE_CODE_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrLocaleInUse, "LOCALE_IN_USE", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidCode, "LOCALE_INVALID_CODE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNameRequired, "LOCALE_NAME_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidFirstDay, "LOCALE_INVALID_FIRST_DAY", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidCountry, "LOCALE_INVALID_COUNTRY", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidSeparators, "LOCALE_INVALID_SEPARATORS", apperr.ClassValidation)
}
