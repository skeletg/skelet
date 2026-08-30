package customer

import "github.com/skeletg/skelet/internal/apperr"

// Pairs every customer-package sentinel with its public code and class, so the GraphQL presenter forwards the message instead of scrubbing it to the generic "internal error" the front surface defaults to
func init() {
	apperr.RegisterSentinel(ErrInvalidCredentials, "INVALID_CREDENTIALS", apperr.ClassAuth)
	apperr.RegisterSentinel(ErrAccountDisabled, "ACCOUNT_DISABLED", apperr.ClassAuth)
	apperr.RegisterSentinel(ErrEmailNotVerified, "EMAIL_NOT_VERIFIED", apperr.ClassAuth)

	apperr.RegisterSentinel(ErrCustomerNotFound, "CUSTOMER_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrNoDefaultGroup, "CUSTOMER_DEFAULT_GROUP_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrEmailTaken, "EMAIL_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrGroupInUse, "CUSTOMER_GROUP_IN_USE", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrGroupSlugTaken, "CUSTOMER_GROUP_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCannotDeleteDefaultGroup, "CUSTOMER_GROUP_DEFAULT", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCustomerHasOrders, "CUSTOMER_HAS_ORDERS", apperr.ClassConflict)
}
