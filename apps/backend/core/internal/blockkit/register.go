package blockkit

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrInvalidDocument, "CMS_INVALID_BLOCK_DOCUMENT", apperr.ClassValidation)
}
