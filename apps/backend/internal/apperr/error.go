// Package apperr is the structured error layer, complementing rather than replacing each domain's typed sentinels - an *Error carries a public Code, a Class, the service-method origin, the file:line of its construction and the wrapped cause, which the GraphQL presenter turns into extensions.code / class / op / origin so the dashboard branches on stable codes and operators trace a 500 back to a line of Go
// At a service boundary: return the bare sentinel when you have one (the presenter looks up its code), else apperr.WrapClass(err, ClassInternal, "domain.Service.Foo")
package apperr

import (
	"errors"
	"runtime"
)

// Construction through New / WrapClass captures the caller's program counter, and Source() resolves it to file:line lazily - Unwrap is implemented, so errors.Is / errors.As still find a wrapped sentinel and every call site branching on domain errors keeps working
type Error struct {
	Code    string // empty when wrapped without an explicit code - the registry then resolves it from the wrapped sentinel
	Class   Class
	Op      string // the service method that produced or annotated this, dotted: "catalog.Service.GetCategory"
	Message string
	Cause   error
	pc      uintptr
}

func (e *Error) Error() string { // "op: message: cause" for CLI tools and logs - the presenter reads the structured fields instead of parsing this
	parts := make([]string, 0, 3)
	if e.Op != "" {
		parts = append(parts, e.Op)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	if e.Cause != nil {
		parts = append(parts, e.Cause.Error())
	}
	return join(parts, ": ")
}

func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) Source() (file string, line int) {
	if e.pc == 0 {
		return "", 0
	}
	frames := runtime.CallersFrames([]uintptr{e.pc})
	frame, _ := frames.Next()
	return frame.File, frame.Line
}

func New(code string, class Class, msg string) *Error {
	return &Error{
		Code:    code,
		Class:   class,
		Message: msg,
		pc:      callerPC(2),
	}
}

// Attaches origin at a service-method boundary without inventing a new public code - the cause chain survives, so errors.Is against sentinels still matches and the presenter resolves a code from the wrapped sentinel
func WrapClass(err error, class Class, op string) *Error {
	if err == nil {
		return nil
	}
	return &Error{
		Class: class,
		Op:    op,
		Cause: err,
		pc:    callerPC(2),
	}
}

func AsAppError(err error) (*Error, bool) {
	if ae, ok := errors.AsType[*Error](err); ok {
		return ae, true
	}
	return nil, false
}

func callerPC(skip int) uintptr {
	var pcs [1]uintptr
	if runtime.Callers(skip+1, pcs[:]) < 1 {
		return 0
	}
	return pcs[0]
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	n := len(sep) * (len(parts) - 1)
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for i, p := range parts {
		if i > 0 {
			out = append(out, sep...)
		}
		out = append(out, p...)
	}
	return string(out)
}
