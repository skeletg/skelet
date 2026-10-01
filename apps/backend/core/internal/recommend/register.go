package recommend

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrInvalidLinkKind, "INVALID_LINK_KIND", apperr.ClassValidation)
}
