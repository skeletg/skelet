package tax

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrRuleNotFound, "TAX_RULE_NOT_FOUND", apperr.ClassNotFound)
}
