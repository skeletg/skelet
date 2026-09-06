package menu

import "github.com/skeletg/skelet/apps/backend/internal/apperr"

// Registers menu sentinels with the apperr registry, so the presenter forwards their messages verbatim instead of scrubbing them to "internal error"
func init() {
	apperr.RegisterSentinel(ErrMenuNotFound, "MENU_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrMenuItemNotFound, "MENU_ITEM_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrMenuSlugTaken, "MENU_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrInvalidTargetType, "MENU_INVALID_TARGET_TYPE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrTargetXorURL, "MENU_TARGET_XOR_URL", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrDepthExceeded, "MENU_DEPTH_EXCEEDED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrParentInOtherMenu, "MENU_PARENT_OTHER_MENU", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrParentCycle, "MENU_PARENT_CYCLE", apperr.ClassValidation)
}
