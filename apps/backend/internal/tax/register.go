package tax

import "github.com/skeletg/skelet/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrRuleNotFound, "TAX_RULE_NOT_FOUND", apperr.ClassNotFound)
}
