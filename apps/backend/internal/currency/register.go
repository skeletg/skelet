package currency

import "github.com/skeletg/skelet/apps/backend/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrCurrencyNotFound, "CURRENCY_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrCurrencyCodeTaken, "CURRENCY_CODE_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCurrencyInUse, "CURRENCY_IN_USE", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidCode, "CURRENCY_INVALID_CODE", apperr.ClassValidation)
}
