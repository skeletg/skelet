package channel

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrChannelNotFound, "CHANNEL_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrSlugTaken, "CHANNEL_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrStoreCurrencyChangeBlocked, "STORE_CURRENCY_CHANGE_BLOCKED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrChannelInUse, "CHANNEL_IN_USE", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrLocaleNotSupported, "CHANNEL_LOCALE_NOT_SUPPORTED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidCurrency, "CHANNEL_INVALID_CURRENCY", apperr.ClassValidation)
}
