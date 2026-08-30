package apperr

import (
	"errors"
	"fmt"
	"sync"
)

type registration struct {
	code  string
	class Class
}

var (
	registryMu sync.RWMutex
	registry   = make(map[error]registration)
)

// Call from a package init() so registration is in place before any request is served - it panics on a nil sentinel or a re-registration with a different code or class, which catches typos at boot
func RegisterSentinel(sentinel error, code string, class Class) {
	if sentinel == nil {
		panic("apperr: RegisterSentinel called with nil sentinel")
	}
	if code == "" {
		panic("apperr: RegisterSentinel called with empty code")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if existing, ok := registry[sentinel]; ok {
		if existing.code != code || existing.class != class {
			panic(fmt.Sprintf(
				"apperr: sentinel %q already registered as %q/%d; refusing to re-register as %q/%d",
				sentinel, existing.code, existing.class, code, class,
			))
		}
		return
	}
	registry[sentinel] = registration{code: code, class: class}
}

// Walks errors.As(*Error) first, since a wrapper may carry an explicit code, then errors.Is against every registered sentinel - "" when nothing matches, which the presenter treats as ClassInternal
func CodeOf(err error) string {
	if err == nil {
		return ""
	}
	if ae, ok := AsAppError(err); ok && ae.Code != "" {
		return ae.Code
	}
	return scanRegistry(err, func(reg registration) string { return reg.code })
}

func ClassOf(err error) Class { // resolves in the same order as CodeOf - *Error.Class when set, else the registered sentinel's class, else ClassUnknown
	if err == nil {
		return ClassUnknown
	}
	if ae, ok := AsAppError(err); ok && ae.Class != ClassUnknown {
		return ae.Class
	}
	var class Class
	scanRegistry(err, func(reg registration) string {
		class = reg.class
		return ""
	})
	return class
}

func scanRegistry(err error, on func(registration) string) string { // fires the callback on the first errors.Is match and forwards its return - O(N sentinels) per call, but only on error paths, so at 150+ sentinels that's microseconds
	registryMu.RLock()
	defer registryMu.RUnlock()
	for sentinel, reg := range registry {
		if errors.Is(err, sentinel) {
			return on(reg)
		}
	}
	return ""
}

func IsNotFound(err error) bool { return ClassOf(err) == ClassNotFound } // for call sites where a miss is a quiet fallback rather than a propagated failure, as in the catalog services' locale-fallback pattern

func IsConflict(err error) bool { return ClassOf(err) == ClassConflict }

func IsValidation(err error) bool { return ClassOf(err) == ClassValidation }

func IsAuth(err error) bool { return ClassOf(err) == ClassAuth }

func IsForbidden(err error) bool { return ClassOf(err) == ClassForbidden }

func IsInternal(err error) bool { return ClassOf(err) == ClassInternal }

func IsUnavailable(err error) bool { return ClassOf(err) == ClassUnavailable }
