package admin

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() { // the TOTP sentinels register next to their declarations in totp.go
	apperr.RegisterSentinel(ErrRoleInUse, "ROLE_IN_USE", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrRoleSlugTaken, "ROLE_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrEmailTaken, "ADMIN_EMAIL_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrAdminNotFound, "ADMIN_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrRoleNotFound, "ROLE_NOT_FOUND", apperr.ClassNotFound)
}
