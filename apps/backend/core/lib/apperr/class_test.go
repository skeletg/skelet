package apperr_test

import (
	"errors"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

var ( // test-local sentinels for the remaining class predicates, owned by the test package so they don't pollute domain registries
	errFakeValidation  = errors.New("fake validation")
	errFakeAuth        = errors.New("fake auth")
	errFakeForbidden   = errors.New("fake forbidden")
	errFakeInternal    = errors.New("fake internal")
	errFakeUnavailable = errors.New("fake unavailable")
)

func init() {
	apperr.RegisterSentinel(errFakeValidation, "FAKE_VALIDATION", apperr.ClassValidation)
	apperr.RegisterSentinel(errFakeAuth, "FAKE_AUTH", apperr.ClassAuth)
	apperr.RegisterSentinel(errFakeForbidden, "FAKE_FORBIDDEN", apperr.ClassForbidden)
	apperr.RegisterSentinel(errFakeInternal, "FAKE_INTERNAL", apperr.ClassInternal)
	apperr.RegisterSentinel(errFakeUnavailable, "FAKE_UNAVAILABLE", apperr.ClassUnavailable)
}

// Each registered sentinel resolves to its own class and to nothing else - an unregistered error and nil are ClassUnknown
func TestClassOfResolvesEveryRegisteredClass(t *testing.T) {
	cases := []struct {
		name string
		hit  error
		want apperr.Class
	}{
		{"validation", errFakeValidation, apperr.ClassValidation},
		{"auth", errFakeAuth, apperr.ClassAuth},
		{"forbidden", errFakeForbidden, apperr.ClassForbidden},
		{"internal", errFakeInternal, apperr.ClassInternal},
		{"unavailable", errFakeUnavailable, apperr.ClassUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := apperr.ClassOf(tc.hit); got != tc.want {
				t.Errorf("ClassOf(%v) = %v, want %v", tc.hit, got, tc.want)
			}
		})
	}
	if got := apperr.ClassOf(errors.New("unregistered")); got != apperr.ClassUnknown {
		t.Errorf("ClassOf(unregistered) = %v, want ClassUnknown", got)
	}
	if got := apperr.ClassOf(nil); got != apperr.ClassUnknown {
		t.Errorf("ClassOf(nil) = %v, want ClassUnknown", got)
	}
}

// Resolves through an apperr.WrapClass wrapper whose own class is ClassUnknown to the wrapped sentinel's class
func TestClassOfResolvesThroughWrapClass(t *testing.T) {
	wrapped := apperr.WrapClass(errFakeValidation, apperr.ClassUnknown, "op")
	if got := apperr.ClassOf(wrapped); got != apperr.ClassValidation {
		t.Fatalf("ClassOf(wrapped) = %v, want ClassValidation", got)
	}
}

// Pins the SCREAMING_SNAKE strings the GraphQL presenter stamps into extensions.class - changing one is a client-visible break
func TestClassString(t *testing.T) {
	cases := []struct {
		class apperr.Class
		want  string
	}{
		{apperr.ClassUnknown, "UNKNOWN"},
		{apperr.ClassNotFound, "NOT_FOUND"},
		{apperr.ClassConflict, "CONFLICT"},
		{apperr.ClassValidation, "VALIDATION"},
		{apperr.ClassAuth, "UNAUTHENTICATED"},
		{apperr.ClassForbidden, "FORBIDDEN"},
		{apperr.ClassInternal, "INTERNAL"},
		{apperr.ClassUnavailable, "UNAVAILABLE"},
		{apperr.ClassRateLimited, "TOO_MANY_REQUESTS"},
		{apperr.Class(250), "UNKNOWN"}, // out-of-range falls to default
	}
	for _, tc := range cases {
		if got := tc.class.String(); got != tc.want {
			t.Errorf("Class(%d).String() = %q, want %q", tc.class, got, tc.want)
		}
	}
}

// Exercises the join helper's edge cases through Error() - a bare wrapper with no op and no message should surface just the cause, and an *Error with nothing at all should be the empty string
func TestErrorEmptyOpAndMessage(t *testing.T) {
	bare := &apperr.Error{Cause: errors.New("root")}
	if got := bare.Error(); got != "root" {
		t.Errorf("bare cause-only Error() = %q, want %q", got, "root")
	}
	empty := &apperr.Error{}
	if got := empty.Error(); got != "" {
		t.Errorf("zero-value Error() = %q, want empty", got)
	}
}

func TestZeroValueSourceIsBlank(t *testing.T) { // confirms Source() returns ("", 0) when no pc was captured, as with an *Error built from a struct literal rather than New or WrapClass
	var e apperr.Error
	file, line := e.Source()
	if file != "" || line != 0 {
		t.Fatalf("zero-value Source() = (%q,%d), want (\"\",0)", file, line)
	}
}

// An error matching two sentinels resolves to the one registered first, every time - a map walk flipped its code and class between calls, and with them whether the front surface scrubbed the message
func TestAnErrorMatchingTwoSentinelsResolvesTheSameEveryTime(t *testing.T) {
	both := errors.Join(errFakeUnavailable, errFakeValidation)
	for range 200 {
		if code, class := apperr.CodeOf(both), apperr.ClassOf(both); code != "FAKE_VALIDATION" || class != apperr.ClassValidation {
			t.Fatalf("CodeOf/ClassOf = %s/%v, want the first-registered FAKE_VALIDATION", code, class)
		}
	}
}
