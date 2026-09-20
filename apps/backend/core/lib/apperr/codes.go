package apperr

// The coarse-grained category of an error - clients branch on Class for behaviour like retry or login, and on Code only when the specific failure mode matters, e.g. telling "slug taken" from "name taken" inside one ClassConflict
type Class uint8

const (
	ClassUnknown Class = iota // the zero value - anything neither registered as a sentinel nor built through apperr lands here, and the GraphQL presenter treats it as ClassInternal for safety
	ClassNotFound
	ClassConflict
	ClassValidation
	ClassAuth
	ClassForbidden
	ClassInternal
	ClassUnavailable
	ClassRateLimited // a per-key quota (login, password reset, vote) - safe to surface verbatim, since the message is operator-authored and never reflects user input
)

func (c Class) String() string {
	switch c {
	case ClassNotFound:
		return "NOT_FOUND"
	case ClassConflict:
		return "CONFLICT"
	case ClassValidation:
		return "VALIDATION"
	case ClassAuth:
		return "UNAUTHENTICATED"
	case ClassForbidden:
		return "FORBIDDEN"
	case ClassInternal:
		return "INTERNAL"
	case ClassUnavailable:
		return "UNAVAILABLE"
	case ClassRateLimited:
		return "TOO_MANY_REQUESTS"
	default:
		return "UNKNOWN"
	}
}
