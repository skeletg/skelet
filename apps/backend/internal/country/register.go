package country

import "github.com/skeletg/skelet/apps/backend/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrCountryNotFound, "COUNTRY_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrCountryCodeTaken, "COUNTRY_CODE_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCountryInUse, "COUNTRY_IN_USE", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidCode, "COUNTRY_INVALID_CODE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrCountryNotAllowed, "COUNTRY_NOT_ALLOWED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrRegionRequired, "REGION_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidRegion, "INVALID_REGION", apperr.ClassValidation)
}
