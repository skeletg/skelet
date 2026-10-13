package graphqlx_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/skeletg/skelet/apps/backend/core/internal/graphqlx"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Test-local sentinels with classifications so the presenter has
// something to look up - Registered in init()
var (
	errFakeNotFound = errors.New("fake not found")
	errFakeBareDB   = errors.New("simulated db crash")
)

func init() {
	apperr.RegisterSentinel(errFakeNotFound, "FAKE_NOT_FOUND_PRES", apperr.ClassNotFound)
}

func TestPresenterAdminRegisteredSentinel(t *testing.T) {
	gqlErr := graphqlx.Presenter(graphqlx.SurfaceAdmin)(context.Background(), errFakeNotFound)
	if gqlErr == nil {
		t.Fatal("presenter returned nil")
	}
	if got := gqlErr.Extensions["code"]; got != "FAKE_NOT_FOUND_PRES" {
		t.Fatalf("code = %v, want FAKE_NOT_FOUND_PRES", got)
	}
	if got := gqlErr.Extensions["class"]; got != "NOT_FOUND" {
		t.Fatalf("class = %v, want NOT_FOUND", got)
	}
	if got := gqlErr.Message; got != "fake not found" {
		t.Fatalf("message = %q, want %q (admin should not scrub)", got, "fake not found")
	}
}

func TestPresenterAdminWrappedWithOrigin(t *testing.T) {
	wrapped := apperr.WrapClass(errFakeBareDB, apperr.ClassInternal, "test.Service.DoThing")
	gqlErr := graphqlx.Presenter(graphqlx.SurfaceAdmin)(context.Background(), wrapped)
	if got := gqlErr.Extensions["op"]; got != "test.Service.DoThing" {
		t.Fatalf("op = %v, want test.Service.DoThing", got)
	}
	origin, _ := gqlErr.Extensions["origin"].(string)
	if !strings.Contains(origin, "presenter_test.go:") {
		t.Fatalf("origin = %q, want suffix presenter_test.go:NN", origin)
	}
	if got := gqlErr.Extensions["class"]; got != "INTERNAL" {
		t.Fatalf("class = %v, want INTERNAL", got)
	}
	if got := gqlErr.Extensions["code"]; got != "INTERNAL" {
		t.Fatalf("code = %v, want INTERNAL (default for unregistered internal)", got)
	}
}

func TestPresenterFrontScrubsInternalMessage(t *testing.T) {
	wrapped := apperr.WrapClass(errFakeBareDB, apperr.ClassInternal, "test.Service.DoThing")
	gqlErr := graphqlx.Presenter(graphqlx.SurfaceFront)(context.Background(), wrapped)
	if gqlErr.Message == "simulated db crash" {
		t.Fatalf("front presenter must not surface internal error message; got %q", gqlErr.Message)
	}
	if gqlErr.Message != "internal error" {
		t.Fatalf("front message = %q, want %q", gqlErr.Message, "internal error")
	}
	if _, ok := gqlErr.Extensions["op"]; ok {
		t.Fatalf("front presenter must not expose op; extensions = %v", gqlErr.Extensions)
	}
	if _, ok := gqlErr.Extensions["origin"]; ok {
		t.Fatalf("front presenter must not expose origin; extensions = %v", gqlErr.Extensions)
	}
	if got := gqlErr.Extensions["class"]; got != "INTERNAL" {
		t.Fatalf("class = %v, want INTERNAL", got)
	}
}

func TestPresenterFrontKeepsNotFoundMessage(t *testing.T) {
	gqlErr := graphqlx.Presenter(graphqlx.SurfaceFront)(context.Background(), errFakeNotFound)
	if gqlErr.Message != "fake not found" {
		t.Fatalf("not-found message should not be scrubbed on front; got %q", gqlErr.Message)
	}
	if got := gqlErr.Extensions["code"]; got != "FAKE_NOT_FOUND_PRES" {
		t.Fatalf("code = %v, want FAKE_NOT_FOUND_PRES", got)
	}
}

func TestPresenterUnregisteredDefaultsToInternal(t *testing.T) {
	gqlErr := graphqlx.Presenter(graphqlx.SurfaceAdmin)(context.Background(), errors.New("random"))
	if got := gqlErr.Extensions["class"]; got != "INTERNAL" {
		t.Fatalf("class = %v, want INTERNAL (default)", got)
	}
	if got := gqlErr.Extensions["code"]; got != "INTERNAL" {
		t.Fatalf("code = %v, want INTERNAL (default)", got)
	}
}

func TestPresenterNilReturnsNil(t *testing.T) {
	if got := graphqlx.Presenter(graphqlx.SurfaceAdmin)(context.Background(), nil); got != nil {
		t.Fatalf("nil err must produce nil gqlerror; got %v", got)
	}
}

// TestPresenterNonInternalEmptyCodeOmitsCodeExtension exercises the branch where the class is known (not internal) but no code resolves: an *Error constructed with an empty Code + ClassValidation - The presenter must stamp class=VALIDATION and leave extensions.code unset (the "INTERNAL" default only fills in for ClassInternal)
func TestPresenterNonInternalEmptyCodeOmitsCodeExtension(t *testing.T) {
	err := apperr.New("", apperr.ClassValidation, "name is required")
	gqlErr := graphqlx.Presenter(graphqlx.SurfaceFront)(context.Background(), err)
	if gqlErr == nil {
		t.Fatal("presenter returned nil")
	}
	if got := gqlErr.Extensions["class"]; got != "VALIDATION" {
		t.Fatalf("class = %v, want VALIDATION", got)
	}
	if _, ok := gqlErr.Extensions["code"]; ok {
		t.Fatalf("code extension should be absent for an empty-code non-internal error; got %v", gqlErr.Extensions["code"])
	}
	// Validation messages are caller-actionable: the front surface keeps
	// them verbatim (only INTERNAL / UNAVAILABLE get scrubbed)
	if gqlErr.Message != "name is required" {
		t.Fatalf("validation message = %q, want it preserved", gqlErr.Message)
	}
}

// TestPresenterFrontScrubsUnavailableMessage confirms ClassUnavailable is scrubbed on the front surface just like
// ClassInternal (both are server-side faults the caller can't act on)
func TestPresenterFrontScrubsUnavailableMessage(t *testing.T) {
	err := apperr.WrapClass(errFakeBareDB, apperr.ClassUnavailable, "test.Service.Dep")
	gqlErr := graphqlx.Presenter(graphqlx.SurfaceFront)(context.Background(), err)
	if gqlErr.Message != "internal error" {
		t.Fatalf("unavailable message = %q, want scrubbed to %q", gqlErr.Message, "internal error")
	}
	if got := gqlErr.Extensions["class"]; got != "UNAVAILABLE" {
		t.Fatalf("class = %v, want UNAVAILABLE", got)
	}
}

func TestRecoverFuncReturnsClassInternalApperr(t *testing.T) {
	err := graphqlx.RecoverFunc(graphqlx.SurfaceAdmin)(context.Background(), "boom")
	if got := apperr.ClassOf(err); got != apperr.ClassInternal {
		t.Fatalf("recover should return ClassInternal; got class %v", got)
	}
	ae, ok := apperr.AsAppError(err)
	if !ok {
		t.Fatal("recover should return *apperr.Error")
	}
	if ae.Op != "graphqlx.RecoverFunc" {
		t.Fatalf("op = %q, want graphqlx.RecoverFunc", ae.Op)
	}
}

func TestPresenterFrontKeepsGraphQLValidationMessages(t *testing.T) {
	for _, code := range []string{errcode.ValidationFailed, errcode.ParseFailed} {
		err := gqlerror.Errorf("variable \"quantity\" is required")
		errcode.Set(err, code)
		gqlErr := graphqlx.Presenter(graphqlx.SurfaceFront)(context.Background(), err)
		if gqlErr.Message != `variable "quantity" is required` {
			t.Errorf("%s: message = %q, want the caller's own validation text", code, gqlErr.Message)
		}
		if got := gqlErr.Extensions["class"]; got != "VALIDATION" {
			t.Errorf("%s: class = %v, want VALIDATION", code, got)
		}
		if got := gqlErr.Extensions["code"]; got != code {
			t.Errorf("code = %v, want %s", got, code)
		}
	}

	// A gqlerror raised while a field resolves is one a resolver built - it stays internal and scrubbed
	inField := graphql.WithFieldContext(context.Background(), &graphql.FieldContext{Field: graphql.CollectedField{}})
	gqlErr := graphqlx.Presenter(graphqlx.SurfaceFront)(inField, gqlerror.Errorf("pq: connection refused"))
	if gqlErr.Message != "internal error" || gqlErr.Extensions["class"] != "INTERNAL" {
		t.Errorf("uncoded gqlerror = %q / %v, want scrubbed INTERNAL", gqlErr.Message, gqlErr.Extensions["class"])
	}
}

type denial struct{}

func (denial) Error() string { return "forbidden" }

func (denial) DenialDetails() (string, string, string) { return "orders.view", "ch-1", "en" }

func TestDenialDetailsOnlyInDev(t *testing.T) {
	for _, dev := range []bool{false, true} {
		gqlErr := graphqlx.Presenter(graphqlx.SurfaceAdmin, graphqlx.WithDevDetails(dev))(context.Background(), denial{})
		_, has := gqlErr.Extensions["denied_permission"]
		if has != dev {
			t.Errorf("dev=%v: denied_permission present = %v", dev, has)
		}
		if dev && (gqlErr.Extensions["denied_channel_id"] != "ch-1" || gqlErr.Extensions["denied_locale"] != "en") {
			t.Errorf("dev details = %v, want the channel and locale too", gqlErr.Extensions)
		}
	}
}

// Only server faults reach the aggregator - a shopper's typo would only fill its dedup index
func TestOnlyServerFaultsAreReported(t *testing.T) {
	rep := &captured{}
	present := graphqlx.Presenter(graphqlx.SurfaceFront, graphqlx.WithReporter(rep))
	_ = present(context.Background(), errFakeNotFound)
	_ = present(context.Background(), apperr.New("", apperr.ClassValidation, "bad"))
	if len(rep.errors) != 0 {
		t.Fatalf("%d caller mistakes reported", len(rep.errors))
	}
	_ = present(context.Background(), errors.New("bare fault"))
	_ = present(context.Background(), apperr.WrapClass(errFakeBareDB, apperr.ClassUnavailable, "test.Dep"))
	if len(rep.errors) != 2 || rep.errors[0].Class != apperr.ClassInternal || rep.errors[1].Class != apperr.ClassUnavailable {
		t.Fatalf("reported %+v, want the internal and the unavailable fault", rep.errors)
	}
}

func TestRecoverReportsThePanic(t *testing.T) {
	rep := &captured{}
	_ = graphqlx.RecoverFunc(graphqlx.SurfaceFront, graphqlx.WithReporter(rep))(context.Background(), "boom")
	if len(rep.panics) != 1 || rep.panics[0] != "boom" {
		t.Fatalf("panics reported = %v, want the raw value once", rep.panics)
	}
}
