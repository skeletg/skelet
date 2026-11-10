package tracking

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

// So a resolver can return these bare and the admin sees why, rather than an internal error
func init() {
	apperr.RegisterSentinel(ErrUnknownKind, "TRACKING_UNKNOWN_KIND", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNameMissing, "TRACKING_NAME_MISSING", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrFieldMissing, "TRACKING_FIELD_MISSING", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrFieldUnknown, "TRACKING_FIELD_UNKNOWN", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNeedsSecret, "TRACKING_NEEDS_SECRET", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNotDead, "TRACKING_DELIVERY_NOT_DEAD", apperr.ClassValidation)
}
