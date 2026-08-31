package admin

import "github.com/skeletg/skelet/apps/backend/internal/apperr"

func init() { // ErrTotpRequired registers next to its declaration in totp.go
	apperr.RegisterSentinel(ErrRoleInUse, "ROLE_IN_USE", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrRoleSlugTaken, "ROLE_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrEmailTaken, "ADMIN_EMAIL_TAKEN", apperr.ClassConflict)
}
