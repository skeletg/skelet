package sdk

import "fmt"

// The front GraphQL presenter scrubs any error it can't classify to "internal error", on the reasoning
// that an unclassified error is a server fault and its text may leak internals - correct by default, and
// useless for "amountCents must be positive", which the caller needs to read
// Returning one of these from a resolver tells the host how to classify it, so the message survives
//
// The message reaches the CLIENT. Never put an upstream response, a config value or a key in it - return
// a flat message and log the detail

type ErrorKind uint8

const (
	// Caller sent something wrong and can fix it - a bad currency code, a negative amount
	ErrorValidation ErrorKind = iota
	// Asked-for thing doesn't exist
	ErrorNotFound
	// Real state conflicts with the request, e.g. already captured
	ErrorConflict
	// Caller isn't allowed - prefer RequirePermission, which returns this shape already
	ErrorForbidden
	// A dependency this plugin needs is down, so the caller may retry
	ErrorUnavailable
)

// Code is an optional stable identifier clients branch on when the Kind alone isn't specific enough,
// conventionally SCREAMING_SNAKE and prefixed with the plugin, e.g. "KLARNA_COUNTRY_UNSUPPORTED"
type UserError struct {
	Kind    ErrorKind
	Code    string
	Message string
}

func (e *UserError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Message
}

// The common case - a caller-fixable input problem
func Invalid(format string, args ...any) error {
	return &UserError{Kind: ErrorValidation, Message: fmt.Sprintf(format, args...)}
}

func Unavailable(format string, args ...any) error {
	return &UserError{Kind: ErrorUnavailable, Message: fmt.Sprintf(format, args...)}
}

// For when the client needs to tell two failures of the same Kind apart
func UserErrorWithCode(kind ErrorKind, code, format string, args ...any) error {
	return &UserError{Kind: kind, Code: code, Message: fmt.Sprintf(format, args...)}
}
