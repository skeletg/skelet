package webhook

import "github.com/skeletg/skelet/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrWebhookNotFound, "WEBHOOK_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrInvalidEventTypes, "WEBHOOK_INVALID_EVENT_TYPES", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidKind, "WEBHOOK_INVALID_KIND", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrKindEventMismatch, "WEBHOOK_KIND_EVENT_MISMATCH", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidFailurePolicy, "WEBHOOK_INVALID_FAILURE_POLICY", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidTimeout, "WEBHOOK_INVALID_TIMEOUT", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrReservedHeader, "WEBHOOK_RESERVED_HEADER", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidPayloadVersion, "WEBHOOK_INVALID_PAYLOAD_VERSION", apperr.ClassValidation)
}
