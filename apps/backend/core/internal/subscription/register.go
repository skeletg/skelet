package subscription

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrNotFound, "SUBSCRIPTION_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrNotOwned, "SUBSCRIPTION_NOT_OWNED", apperr.ClassForbidden)
	apperr.RegisterSentinel(ErrNotActive, "SUBSCRIPTION_NOT_ACTIVE", apperr.ClassConflict)
}
