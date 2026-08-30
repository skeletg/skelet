package apperr_test

import (
	"errors"
	"testing"

	"github.com/skeletg/skelet/internal/apperr"
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

// Covers the class predicates TestIsHelpers doesn't, since that only checks IsNotFound and IsConflict - each must be true for its own class and false both for a different one and for an unregistered error
func TestRemainingIsHelpers(t *testing.T) {
	cases := []struct {
		name string
		fn   func(error) bool
		hit  error // resolves true
		miss error // resolves false
	}{
		{"IsValidation", apperr.IsValidation, errFakeValidation, errFakeAuth},
		{"IsAuth", apperr.IsAuth, errFakeAuth, errFakeValidation},
		{"IsForbidden", apperr.IsForbidden, errFakeForbidden, errFakeAuth},
		{"IsInternal", apperr.IsInternal, errFakeInternal, errFakeForbidden},
		{"IsUnavailable", apperr.IsUnavailable, errFakeUnavailable, errFakeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.fn(tc.hit) {
				t.Errorf("%s(%v) = false, want true", tc.name, tc.hit)
			}
			if tc.fn(tc.miss) {
				t.Errorf("%s(%v) = true, want false", tc.name, tc.miss)
			}
			if tc.fn(errors.New("unregistered")) {
				t.Errorf("%s(unregistered) = true, want false", tc.name)
			}
			if tc.fn(nil) {
				t.Errorf("%s(nil) = true, want false", tc.name)
			}
		})
	}
}

// Confirms the predicates resolve through an apperr.WrapClass wrapper whose own class differs from the sentinel's - WrapClass sets ClassUnknown on itself, so ClassOf falls back to the wrapped sentinel's class
func TestIsHelpersThroughWrapClass(t *testing.T) {
	wrapped := apperr.WrapClass(errFakeValidation, apperr.ClassUnknown, "op")
	if !apperr.IsValidation(wrapped) {
		t.Fatalf("IsValidation should resolve through a ClassUnknown WrapClass to the sentinel")
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
