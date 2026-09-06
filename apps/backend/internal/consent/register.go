package consent

import "github.com/skeletg/skelet/apps/backend/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrUnknownConsent, "CONSENT_UNKNOWN_KIND", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrConsentRequired, "CONSENT_REQUIRED", apperr.ClassValidation)
}
