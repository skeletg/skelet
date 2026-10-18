package apperr_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Sentinels owned by the test package so we don't pollute domain
// registries - Registered in init() below
var (
	errFakeNotFound  = errors.New("fake not found")
	errFakeConflict  = errors.New("fake conflict")
	errUnregistered  = errors.New("unregistered")
	errBareInfraFail = errors.New("simulated db down")
)

func init() {
	apperr.RegisterSentinel(errFakeNotFound, "FAKE_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(errFakeConflict, "FAKE_CONFLICT", apperr.ClassConflict)
}

func TestErrorUnwrapPreservesSentinel(t *testing.T) {
	wrapped := apperr.WrapClass(errFakeNotFound, apperr.ClassNotFound, "test.Op")
	if !errors.Is(wrapped, errFakeNotFound) {
		t.Fatalf("errors.Is should still match wrapped sentinel")
	}
}

func TestWrapClassNilReturnsNil(t *testing.T) {
	if got := apperr.WrapClass(nil, apperr.ClassInternal, "test.Op"); got != nil {
		t.Fatalf("WrapClass(nil) = %v, want nil", got)
	}
}

func TestSourceCapturesCallerFileLine(t *testing.T) {
	e := apperr.WrapClass(errFakeNotFound, apperr.ClassNotFound, "test.Op")
	file, line := e.Source()
	if !strings.HasSuffix(file, "error_test.go") {
		t.Fatalf("Source file = %q, want suffix error_test.go", file)
	}
	if line == 0 {
		t.Fatalf("Source line = 0, want non-zero")
	}
}

func TestNewSourceMatchesNewCallSite(t *testing.T) {
	e := apperr.New("X", apperr.ClassInternal, "boom")
	file, _ := e.Source()
	if !strings.HasSuffix(file, "error_test.go") {
		t.Fatalf("Source file = %q, want suffix error_test.go", file)
	}
}

func TestCodeOf(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"registered sentinel direct", errFakeNotFound, "FAKE_NOT_FOUND"},
		{"registered sentinel wrapped via fmt.Errorf", fmt.Errorf("ctx: %w", errFakeNotFound), "FAKE_NOT_FOUND"},
		{"registered sentinel wrapped via WrapClass", apperr.WrapClass(errFakeNotFound, apperr.ClassNotFound, "op"), "FAKE_NOT_FOUND"},
		{"unregistered sentinel", errUnregistered, ""},
		{"explicit code via New", apperr.New("CUSTOM", apperr.ClassConflict, "boom"), "CUSTOM"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := apperr.CodeOf(tc.err); got != tc.want {
				t.Fatalf("CodeOf = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClassOf(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want apperr.Class
	}{
		{"nil", nil, apperr.ClassUnknown},
		{"sentinel direct", errFakeNotFound, apperr.ClassNotFound},
		{"sentinel wrapped", fmt.Errorf("%w", errFakeNotFound), apperr.ClassNotFound},
		{"WrapClass overrides to internal", apperr.WrapClass(errFakeNotFound, apperr.ClassInternal, "op"), apperr.ClassInternal},
		{"unregistered", errUnregistered, apperr.ClassUnknown},
		{"new with class", apperr.New("X", apperr.ClassValidation, ""), apperr.ClassValidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := apperr.ClassOf(tc.err); got != tc.want {
				t.Fatalf("ClassOf = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsHelpers(t *testing.T) {
	if !apperr.IsNotFound(errFakeNotFound) {
		t.Fatalf("IsNotFound(errFakeNotFound) should be true")
	}
	if apperr.IsNotFound(errFakeConflict) {
		t.Fatalf("IsNotFound(errFakeConflict) should be false")
	}
	if apperr.ClassOf(errFakeConflict) != apperr.ClassConflict {
		t.Fatalf("ClassOf(errFakeConflict) should be ClassConflict")
	}
	if apperr.IsNotFound(errUnregistered) {
		t.Fatalf("IsNotFound(unregistered) should be false")
	}
}

func TestRegisterSentinelDuplicateSameCodeIsIdempotent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("re-registering with same code panicked: %v", r)
		}
	}()
	apperr.RegisterSentinel(errFakeNotFound, "FAKE_NOT_FOUND", apperr.ClassNotFound)
}

func TestRegisterSentinelDuplicateDifferentCodePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic when re-registering sentinel with different code")
		}
	}()
	apperr.RegisterSentinel(errFakeNotFound, "OTHER_CODE", apperr.ClassNotFound)
}

func TestRegisterSentinelNilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on nil sentinel")
		}
	}()
	apperr.RegisterSentinel(nil, "X", apperr.ClassNotFound)
}

func TestRegisterSentinelEmptyCodePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on empty code")
		}
	}()
	apperr.RegisterSentinel(errBareInfraFail, "", apperr.ClassInternal)
}

func TestErrorString(t *testing.T) {
	e := apperr.WrapClass(errFakeNotFound, apperr.ClassNotFound, "catalog.Service.GetCategory")
	got := e.Error()
	if !strings.Contains(got, "catalog.Service.GetCategory") {
		t.Fatalf("Error() = %q, want to contain op", got)
	}
	if !strings.Contains(got, "fake not found") {
		t.Fatalf("Error() = %q, want to contain cause", got)
	}
}

func TestErrorsAsExtractsAppError(t *testing.T) {
	wrapped := apperr.WrapClass(errFakeNotFound, apperr.ClassNotFound, "op")
	via := fmt.Errorf("outer: %w", wrapped)
	ae, ok := apperr.AsAppError(via)
	if !ok {
		t.Fatal("AsAppError should find the wrapped *Error")
	}
	if ae.Op != "op" {
		t.Fatalf("Op = %q, want %q", ae.Op, "op")
	}
}
