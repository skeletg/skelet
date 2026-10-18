package channel

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrChannelNotFound, "CHANNEL_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrSlugTaken, "CHANNEL_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrStoreCurrencyChangeBlocked, "STORE_CURRENCY_CHANGE_BLOCKED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrChannelInUse, "CHANNEL_IN_USE", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrStorefrontHostTaken, "CHANNEL_STOREFRONT_HOST_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrDefaultChannelInactive, "CHANNEL_DEFAULT_INACTIVE", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrDeactivateDefaultChannel, "CHANNEL_DEFAULT_DEACTIVATE", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidCurrency, "CHANNEL_INVALID_CURRENCY", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidSlug, "CHANNEL_INVALID_SLUG", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNameRequired, "CHANNEL_NAME_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidPublicURL, "CHANNEL_INVALID_PUBLIC_URL", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrMetadataTooLarge, "CHANNEL_METADATA_TOO_LARGE", apperr.ClassValidation)
}
