package currency

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrCurrencyNotFound, "CURRENCY_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrCurrencyCodeTaken, "CURRENCY_CODE_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCurrencyInUse, "CURRENCY_IN_USE", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrDecimalsLocked, "CURRENCY_DECIMALS_LOCKED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCurrencyUsedByChannel, "CURRENCY_USED_BY_CHANNEL", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidCode, "CURRENCY_INVALID_CODE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNameRequired, "CURRENCY_NAME_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidSymbolPosition, "CURRENCY_INVALID_SYMBOL_POSITION", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidDecimals, "CURRENCY_INVALID_DECIMALS", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidRate, "CURRENCY_INVALID_RATE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrMetadataTooLarge, "CURRENCY_METADATA_TOO_LARGE", apperr.ClassValidation)

	apperr.RegisterSentinel(ErrRateBaseMismatch, "CURRENCY_RATE_BASE_MISMATCH", apperr.ClassUnavailable)
}
