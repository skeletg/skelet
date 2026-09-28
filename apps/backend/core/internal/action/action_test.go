package action

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func noopHandler(context.Context, Args) (Result, error) { return Result{}, nil }

// The registry is a package global with no unregister, so every test that touches it puts the previous contents back
func isolateRegistry(t *testing.T) {
	t.Helper()
	mu.Lock()
	saved := make([]Definition, len(registry))
	copy(saved, registry)
	registry = nil
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		registry = saved
		mu.Unlock()
	})
}

func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic, want a registration panic", what)
		}
	}()
	fn()
}

func TestValidateDefinitionRejectsMalformedRegistrations(t *testing.T) {
	base := Definition{Slug: "test.ok", Name: "Ok", Permission: "test.run", Handler: noopHandler}
	if err := validateDefinition(base); err != nil {
		t.Fatalf("the baseline definition is rejected: %v", err)
	}

	for name, mutate := range map[string]func(*Definition){
		"empty slug":         func(d *Definition) { d.Slug = "" },
		"uppercase slug":     func(d *Definition) { d.Slug = "Test.Ok" },
		"slug with a space":  func(d *Definition) { d.Slug = "test ok" },
		"slug with a tab":    func(d *Definition) { d.Slug = "test\tok" },
		"blank name":         func(d *Definition) { d.Name = "   " },
		"missing permission": func(d *Definition) { d.Permission = "" },
		"nil handler":        func(d *Definition) { d.Handler = nil },
		"unnamed argument": func(d *Definition) {
			d.Arguments = []Argument{{Type: ArgString}}
		},
		"duplicate argument": func(d *Definition) {
			d.Arguments = []Argument{{Name: "n", Type: ArgInt}, {Name: "n", Type: ArgString}}
		},
		"select without options": func(d *Definition) {
			d.Arguments = []Argument{{Name: "mode", Type: ArgSelect}}
		},
	} {
		d := base
		mutate(&d)
		if err := validateDefinition(d); err == nil {
			t.Errorf("%s was accepted, want a validation error", name)
		}
	}
}

// Two packages claiming one slug means the second silently shadows the first in Find - loud at boot beats an admin running the wrong operation
func TestRegisterPanicsOnDuplicateSlugAndOnInvalidDefinitions(t *testing.T) {
	isolateRegistry(t)
	d := Definition{Slug: "test.dupe", Name: "Dupe", Permission: "test.run", Handler: noopHandler}
	Register(d)
	mustPanic(t, "a second Register of the same slug", func() { Register(d) })
	mustPanic(t, "Register of a handler-less definition", func() {
		Register(Definition{Slug: "test.nohandler", Name: "N", Permission: "test.run"})
	})
	if all := AllDefinitions(); len(all) != 1 {
		t.Errorf("registry holds %d definitions, want the 1 that registered cleanly", len(all))
	}
}

// AllDefinitions hands the slice to the admin UI, so a caller mutating it must not corrupt the registry
func TestAllDefinitionsReturnsACopy(t *testing.T) {
	isolateRegistry(t)
	Register(Definition{Slug: "test.copy", Name: "Copy", Permission: "test.run", Handler: noopHandler})
	all := AllDefinitions()
	all[0].Name = "Clobbered"
	if got, _ := Find("test.copy"); got.Name != "Copy" {
		t.Errorf("registry name = %q after a caller edited the returned slice, want %q", got.Name, "Copy")
	}
}

func TestFindMissesUnknownSlugs(t *testing.T) {
	isolateRegistry(t)
	if _, ok := Find("test.never-registered"); ok {
		t.Error("Find on an unregistered slug = true, want false")
	}
}

type stubChecker struct {
	granted map[string]bool
	err     error
}

func (s stubChecker) HasPermission(_ context.Context, _ uuid.UUID, slug string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.granted[slug], nil
}

// The Tools page lists only what the caller may actually run, and a checker error has to hide the action rather than expose it
func TestAllAvailableFiltersByPermission(t *testing.T) {
	isolateRegistry(t)
	Register(Definition{Slug: "test.allowed", Name: "A", Permission: "perm.yes", Handler: noopHandler})
	Register(Definition{Slug: "test.denied", Name: "D", Permission: "perm.no", Handler: noopHandler})

	got := AllAvailable(context.Background(), stubChecker{granted: map[string]bool{"perm.yes": true}}, uuid.New())
	if len(got) != 1 || got[0].Slug != "test.allowed" {
		t.Errorf("AllAvailable = %v, want only test.allowed", slugsOf(got))
	}

	got = AllAvailable(context.Background(), stubChecker{err: errors.New("permission backend down")}, uuid.New())
	if len(got) != 0 {
		t.Errorf("AllAvailable with a failing checker = %v, want nothing listed", slugsOf(got))
	}
}

func slugsOf(defs []Definition) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Slug)
	}
	return out
}

// Args come off a GraphQL JSON map, so an unexpected key means the caller and the schema disagree - dropping it silently would run the action with arguments the admin thinks they supplied
func TestValidateArgsRejectsUnknownAndMissingArguments(t *testing.T) {
	schema := []Argument{{Name: "days", Type: ArgInt, Required: true}}

	if _, err := validateArgs(schema, map[string]any{"days": 5, "typo": 1}); err == nil {
		t.Error("an unknown argument was accepted, want it refused")
	}
	if _, err := validateArgs(schema, map[string]any{}); err == nil {
		t.Error("a missing required argument was accepted, want it refused")
	}
	if _, err := validateArgs(schema, map[string]any{"days": 5}); err != nil {
		t.Errorf("the well-formed call = %v, want nil", err)
	}
}

// JSON numbers arrive as float64 - the whole reason handlers read through Args instead of the raw map
func TestValidateArgsCoercesEachType(t *testing.T) {
	schema := []Argument{
		{Name: "name", Type: ArgString},
		{Name: "count", Type: ArgInt},
		{Name: "dry", Type: ArgBool},
		{Name: "mode", Type: ArgSelect, Options: []ArgumentOption{{Value: "fast"}, {Value: "slow"}}},
	}
	out, err := validateArgs(schema, map[string]any{
		"name": "sweep", "count": float64(42), "dry": true, "mode": "slow",
	})
	if err != nil {
		t.Fatalf("validateArgs: %v", err)
	}
	if got, ok := out["count"].(int64); !ok || got != 42 {
		t.Errorf("count = %#v, want int64(42) - handlers type-assert on int64", out["count"])
	}
	if out["name"] != "sweep" || out["dry"] != true || out["mode"] != "slow" {
		t.Errorf("coerced args = %#v, want the inputs preserved", out)
	}
}

func TestValidateArgsRejectsWrongTypesAndBadSelectValues(t *testing.T) {
	for name, tc := range map[string]struct {
		arg Argument
		val any
	}{
		"string given a number":  {Argument{Name: "a", Type: ArgString}, 1},
		"bool given a string":    {Argument{Name: "a", Type: ArgBool}, "true"},
		"int given a bool":       {Argument{Name: "a", Type: ArgInt}, true},
		"int given a non-number": {Argument{Name: "a", Type: ArgInt}, "seven"},
		"select given a number":  {Argument{Name: "a", Type: ArgSelect, Options: []ArgumentOption{{Value: "x"}}}, 1},
		"select off the list":    {Argument{Name: "a", Type: ArgSelect, Options: []ArgumentOption{{Value: "x"}}}, "y"},
		"unknown type":           {Argument{Name: "a", Type: ArgumentType("date")}, "2026-08-16"},
	} {
		if _, err := validateArgs([]Argument{tc.arg}, map[string]any{"a": tc.val}); err == nil {
			t.Errorf("%s was accepted, want a coercion error", name)
		}
	}
}

// An int arrives as a string when the admin form posts one - accepted, but still normalized to int64
func TestValidateArgsAcceptsNumericStringsForInts(t *testing.T) {
	out, err := validateArgs([]Argument{{Name: "n", Type: ArgInt}}, map[string]any{"n": "90"})
	if err != nil {
		t.Fatalf("numeric string for an int arg: %v", err)
	}
	if got, ok := out["n"].(int64); !ok || got != 90 {
		t.Errorf("n = %#v, want int64(90)", out["n"])
	}
}

// An omitted optional argument only lands in the bag when it has a default - handlers distinguish "not supplied" from "supplied as zero" through the ok return
func TestValidateArgsAppliesDefaultsOnlyWhenSet(t *testing.T) {
	schema := []Argument{
		{Name: "days", Type: ArgInt, Default: "30"},
		{Name: "dry", Type: ArgBool, Default: "true"},
		{Name: "mode", Type: ArgSelect, Default: "fast", Options: []ArgumentOption{{Value: "fast"}}},
		{Name: "note", Type: ArgString}, // no default
	}
	out, err := validateArgs(schema, map[string]any{})
	if err != nil {
		t.Fatalf("validateArgs: %v", err)
	}
	if got, ok := out["days"].(int64); !ok || got != 30 {
		t.Errorf("days default = %#v, want int64(30)", out["days"])
	}
	if out["dry"] != true || out["mode"] != "fast" {
		t.Errorf("defaults = %#v, want dry=true mode=fast", out)
	}
	if _, present := out["note"]; present {
		t.Errorf("note = %#v, want it absent so Args.String reports not-supplied", out["note"])
	}

	// an explicit value always beats the default
	out, err = validateArgs(schema, map[string]any{"days": float64(7)})
	if err != nil {
		t.Fatalf("validateArgs with an explicit value: %v", err)
	}
	if got := out["days"].(int64); got != 7 {
		t.Errorf("days = %d, want the supplied 7 rather than the default", got)
	}
}

// A default that can't parse is a registration bug, and it surfaces the first time the action runs
func TestValidateArgsRejectsUnparseableDefaults(t *testing.T) {
	for name, arg := range map[string]Argument{
		"int default":  {Name: "n", Type: ArgInt, Default: "many"},
		"bool default": {Name: "b", Type: ArgBool, Default: "yes-please"},
		"unknown type": {Name: "d", Type: ArgumentType("date"), Default: "2026-08-16"},
	} {
		if _, err := validateArgs([]Argument{arg}, map[string]any{}); err == nil {
			t.Errorf("%s %q was accepted, want a parse error", name, arg.Default)
		}
	}
}

func TestArgsAccessors(t *testing.T) {
	a := Args{raw: map[string]any{
		"s": "text", "f": float64(3), "i": 4, "i64": int64(5), "numstr": "6", "b": true, "bad": []string{},
	}}

	if got, ok := a.String("s"); !ok || got != "text" {
		t.Errorf(`String("s") = (%q, %v), want ("text", true)`, got, ok)
	}
	if _, ok := a.String("missing"); ok {
		t.Error(`String("missing") = ok, want false`)
	}
	if _, ok := a.String("f"); ok {
		t.Error(`String on a number = ok, want false`)
	}
}

func TestRunRejectsUnknownActions(t *testing.T) {
	isolateRegistry(t)
	if _, err := NewService().Run(context.Background(), "test.nope", nil); !errors.Is(err, ErrUnknownAction) {
		t.Errorf("Run on an unregistered slug = %v, want ErrUnknownAction", err)
	}
}

// Bad arguments must never reach the handler - the wrapped error is what the resolver shows the admin
func TestRunValidatesBeforeCallingTheHandler(t *testing.T) {
	isolateRegistry(t)
	var ran bool
	Register(Definition{
		Slug: "test.guarded", Name: "Guarded", Permission: "test.run",
		Arguments: []Argument{{Name: "days", Type: ArgInt, Required: true}},
		Handler: func(context.Context, Args) (Result, error) {
			ran = true
			return Result{}, nil
		},
	})

	_, err := NewService().Run(context.Background(), "test.guarded", map[string]any{})
	if !errors.Is(err, ErrInvalidArgs) {
		t.Fatalf("Run without the required argument = %v, want ErrInvalidArgs", err)
	}
	if !strings.Contains(err.Error(), "days") {
		t.Errorf("error = %v, want it to name the offending argument", err)
	}
	if ran {
		t.Error("the handler ran despite invalid arguments")
	}
}

// A handler error is a failed run, not a transport error - Run passes both the result and the error back
func TestRunReturnsTheHandlerResultAndError(t *testing.T) {
	isolateRegistry(t)
	boom := errors.New("nothing to prune")
	Register(Definition{
		Slug: "test.result", Name: "Result", Permission: "test.run",
		Arguments: []Argument{{Name: "days", Type: ArgInt, Default: "30"}},
		Handler: func(_ context.Context, args Args) (Result, error) {
			days, ok := args.raw["days"].(int64)
			if !ok || days != 30 {
				return Result{}, errors.New("handler did not receive the coerced default")
			}
			return Result{Output: "pruned 0 rows"}, boom
		},
	})

	res, err := NewService().Run(context.Background(), "test.result", nil)
	if !errors.Is(err, boom) {
		t.Errorf("Run error = %v, want the handler's %v", err, boom)
	}
	if res.Output != "pruned 0 rows" {
		t.Errorf("Output = %q, want the handler's output preserved alongside the error", res.Output)
	}
}

// Destructive actions declare their own budget, and the handler has to actually see it - an unbounded sweep holding a request open is the failure this prevents
func TestRunAppliesTheDeclaredTimeout(t *testing.T) {
	isolateRegistry(t)
	var deadline time.Duration
	Register(Definition{
		Slug: "test.timeout", Name: "Timeout", Permission: "test.run", Timeout: 250 * time.Millisecond,
		Handler: func(ctx context.Context, _ Args) (Result, error) {
			d, ok := ctx.Deadline()
			if !ok {
				return Result{}, errors.New("handler ctx carried no deadline")
			}
			deadline = time.Until(d)
			return Result{}, nil
		},
	})
	if _, err := NewService().Run(context.Background(), "test.timeout", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if deadline <= 0 || deadline > 250*time.Millisecond {
		t.Errorf("handler deadline = %v, want it inside the declared 250ms", deadline)
	}
}

func TestRunFallsBackToTheDefaultTimeout(t *testing.T) {
	isolateRegistry(t)
	var deadline time.Duration
	Register(Definition{
		Slug: "test.deftimeout", Name: "Default timeout", Permission: "test.run",
		Handler: func(ctx context.Context, _ Args) (Result, error) {
			d, _ := ctx.Deadline()
			deadline = time.Until(d)
			return Result{}, nil
		},
	})
	if _, err := NewService().Run(context.Background(), "test.deftimeout", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if deadline <= defaultTimeout-time.Second || deadline > defaultTimeout {
		t.Errorf("handler deadline = %v, want roughly the %v default", deadline, defaultTimeout)
	}
}
