package payment

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrProviderNotFound, "PAYMENT_PROVIDER_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrProviderNotInstalled, "PAYMENT_PROVIDER_NOT_INSTALLED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrProviderDisabled, "PAYMENT_PROVIDER_DISABLED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrSyncUnsupported, "PAYMENT_SYNC_UNSUPPORTED", apperr.ClassValidation)
}
