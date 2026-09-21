package remoteprovider

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrNotFound, "REMOTE_PROVIDER_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrSlugTaken, "REMOTE_PROVIDER_SLUG_TAKEN", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidKind, "REMOTE_PROVIDER_INVALID_KIND", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidSlug, "REMOTE_PROVIDER_INVALID_SLUG", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidTimeout, "REMOTE_PROVIDER_INVALID_TIMEOUT", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidCacheTTL, "REMOTE_PROVIDER_INVALID_CACHE_TTL", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidPolicy, "REMOTE_PROVIDER_INVALID_POLICY", apperr.ClassValidation)

	apperr.RegisterSentinel(ErrCircuitOpen, "REMOTE_PROVIDER_CIRCUIT_OPEN", apperr.ClassUnavailable)
}
