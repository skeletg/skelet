package front

import "github.com/skeletg/skelet/apps/backend/internal/apperr" // caller-input errors are classified here because unclassified is the WRONG default - the front presenter maps anything unregistered to ClassInternal, so a shopper's malformed cart id showed "internal error" and paged the operator for their own typo

func invalidID(what string) error { // `what` names the thing ("cart", "product") so one code serves every id and the message still says which - empty for generic helpers that parse an id blind
	msg := "invalid id"
	if what != "" {
		msg = "invalid " + what + " id"
	}
	return apperr.New("INVALID_ID", apperr.ClassValidation, msg)
}

func storeRequired() error { // no channel resolved - the caller omitted X-Channel-Id, or named one that isn't active
	return apperr.New("STORE_REQUIRED", apperr.ClassValidation, "store required")
}

func selectorRequired(msg string) error { // a lookup that needs one of several selectors and got none
	return apperr.New("SELECTOR_REQUIRED", apperr.ClassValidation, msg)
}

func errNotConfigured(subsystem string) error { // a subsystem the deployment didn't wire (uploads, promotions) - unavailable rather than broken
	return apperr.New("NOT_CONFIGURED", apperr.ClassUnavailable, subsystem+" is not configured")
}
