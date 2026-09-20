package admin

import (
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Resolver-level error shapes. Bare fmt.Errorf reaches the presenter unclassified, which masks it as INTERNAL and logs it at Error - so a mistyped UUID from the dashboard looked like a server fault.
// These four cover what resolvers actually produce that isn't already a domain sentinel

// errInvalidID is a malformed identifier in the request (a UUID that won't parse) - the client sent it, so it's VALIDATION
func errInvalidID(field string, err error) error {
	return &apperr.Error{
		Code:    "INVALID_ID",
		Class:   apperr.ClassValidation,
		Message: fmt.Sprintf("%s is not a valid id", field),
		Cause:   err,
	}
}

// errInvalidInput is a malformed request payload that isn't an identifier - a JSON field that won't decode, a bad token
func errInvalidInput(field string, err error) error {
	return &apperr.Error{
		Code:    "INVALID_INPUT",
		Class:   apperr.ClassValidation,
		Message: fmt.Sprintf("%s is not valid", field),
		Cause:   err,
	}
}

// errNotFound is the resolver-side "nothing under this id" answer, for lookups the service reports with a bool rather than a sentinel
func errNotFound(entity string) error {
	return apperr.New("NOT_FOUND", apperr.ClassNotFound, entity+" not found")
}

// errNotConfigured is a subsystem that isn't wired in this deployment (ai, segments, scheduler) - the operation is unavailable, not broken
func errNotConfigured(subsystem string) error {
	return apperr.New("NOT_CONFIGURED", apperr.ClassUnavailable, subsystem+" is not configured")
}

// errAdminRequired is a resolver reached without an admin identity in context - the authz middleware is non-blocking, so resolvers check for themselves
func errAdminRequired() error {
	return apperr.New("ADMIN_CONTEXT_REQUIRED", apperr.ClassAuth, "admin context required")
}
