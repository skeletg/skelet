package address

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrUnavailable, "ADDRESS_VALIDATION_UNAVAILABLE", apperr.ClassUnavailable)
}
