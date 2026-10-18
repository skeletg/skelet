package dataio

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func testResource(imp func(ctx context.Context, r *Row) (Outcome, error)) Resource {
	return Resource{
		Slug:             "widgets",
		Label:            "Widgets",
		Group:            "Test",
		ViewPermission:   "test.view",
		ImportPermission: "test.manage",
		Fields: []Field{
			{Name: "sku", Type: FieldText, Identifier: true, Required: true},
			{Name: "name", Type: FieldText},
			{Name: "price", Type: FieldMoney, Example: "1999"},
			{Name: "active", Type: FieldBool},
		},
		DynamicPrefixes: []string{"attr:"},
		NewImporter: func(context.Context, Opts) (Importer, error) {
			return importerFunc(imp), nil
		},
	}
}

type importerFunc func(ctx context.Context, r *Row) (Outcome, error)

func (f importerFunc) Row(ctx context.Context, r *Row) (Outcome, error) { return f(ctx, r) }

func newTestRegistry(t *testing.T, res Resource) *Registry {
	t.Helper()
	reg := NewRegistry()
	reg.Register(res)
	return reg
}

func runImport(t *testing.T, reg *Registry, format Format, body string) (ImportResult, error) {
	t.Helper()
	return reg.Import(context.Background(), Run{Resource: "widgets", Format: format}, strings.NewReader(body))
}

func TestImportCountsOutcomesPerRow(t *testing.T) {
	reg := newTestRegistry(t, testResource(func(_ context.Context, r *Row) (Outcome, error) {
		switch r.Str("sku") {
		case "NEW":
			return OutcomeCreated, nil
		case "OLD":
			return OutcomeUpdated, nil
		case "BAD":
			return OutcomeSkipped, errors.New("nope")
		default:
			return OutcomeSkipped, nil
		}
	}))
	res, err := runImport(t, reg, FormatCSV, "sku,name\nNEW,a\nOLD,b\nBAD,c\nMEH,d\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Total != 4 || res.Created != 1 || res.Updated != 1 || res.Failed != 1 || res.Skipped != 1 {
		t.Fatalf("unexpected counts: %+v", res)
	}
	if len(res.Errors) != 1 || res.Errors[0].Line != 4 {
		t.Fatalf("want one error on line 4, got %+v", res.Errors)
	}
}

// A row that ignores a rejected cell must still fail - otherwise a bad price silently imports as 0
func TestImportFailsRowOnUnreadCellError(t *testing.T) {
	reg := newTestRegistry(t, testResource(func(_ context.Context, r *Row) (Outcome, error) {
		r.Int("price")
		return OutcomeCreated, nil
	}))
	res, err := runImport(t, reg, FormatCSV, "sku,price\nA,notanumber\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Failed != 1 || res.Created != 0 {
		t.Fatalf("want the row failed, got %+v", res)
	}
	if !strings.Contains(res.Errors[0].Message, "price") {
		t.Fatalf("error should name the column: %q", res.Errors[0].Message)
	}
}

// Every bad cell in one message, so a fix-and-retry loop is one round trip not four
func TestRowAccumulatesEveryBadCell(t *testing.T) {
	reg := newTestRegistry(t, testResource(func(_ context.Context, r *Row) (Outcome, error) {
		r.Int("price")
		r.OptBool("active")
		return OutcomeCreated, nil
	}))
	res, _ := runImport(t, reg, FormatCSV, "sku,price,active\nA,x,maybe\n")
	msg := res.Errors[0].Message
	if !strings.Contains(msg, "price") || !strings.Contains(msg, "active") {
		t.Fatalf("want both columns in one message, got %q", msg)
	}
}

func TestImportPanicIsolatedToItsRow(t *testing.T) {
	reg := newTestRegistry(t, testResource(func(_ context.Context, r *Row) (Outcome, error) {
		if r.Str("sku") == "BOOM" {
			panic("kaboom")
		}
		return OutcomeCreated, nil
	}))
	res, err := runImport(t, reg, FormatCSV, "sku\nA\nBOOM\nB\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Created != 2 || res.Failed != 1 {
		t.Fatalf("a panicking row should not take the file down: %+v", res)
	}
}

func TestImportRejectsFileMissingIdentifier(t *testing.T) {
	reg := newTestRegistry(t, testResource(func(context.Context, *Row) (Outcome, error) { return OutcomeCreated, nil }))
	if _, err := runImport(t, reg, FormatCSV, "name,price\nx,1\n"); err == nil {
		t.Fatal("want a hard failure when the upsert key column is absent")
	}
}

func TestImportWarnsOnUnknownColumnButKeepsPrefixed(t *testing.T) {
	reg := newTestRegistry(t, testResource(func(context.Context, *Row) (Outcome, error) { return OutcomeCreated, nil }))
	res, err := runImport(t, reg, FormatCSV, "sku,attr:color,nonsense\nA,red,x\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "nonsense") {
		t.Fatalf("want one warning about the unknown column, got %+v", res.Warnings)
	}
}

// Pass grouping is what lets a child row depend on a parent row further down the file
func TestImportRunsPassesInOrder(t *testing.T) {
	var order []string
	res := testResource(func(_ context.Context, r *Row) (Outcome, error) {
		order = append(order, r.Str("sku"))
		return OutcomeCreated, nil
	})
	res.NewImporter = func(context.Context, Opts) (Importer, error) {
		return twoPassImporter{fn: func(_ context.Context, r *Row) (Outcome, error) {
			order = append(order, r.Str("sku"))
			return OutcomeCreated, nil
		}}, nil
	}
	reg := newTestRegistry(t, res)
	if _, err := runImport(t, reg, FormatCSV, "sku,name\nCHILD,child\nPARENT,parent\n"); err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(order) != 2 || order[0] != "PARENT" {
		t.Fatalf("parents must run before children, got %v", order)
	}
}

type twoPassImporter struct{ fn importerFunc }

func (i twoPassImporter) Row(ctx context.Context, r *Row) (Outcome, error) { return i.fn(ctx, r) }

func (i twoPassImporter) Pass(r *Row) int {
	if strings.HasPrefix(r.Str("sku"), "CHILD") {
		return 1
	}
	return 0
}

func TestFinisherRunsAfterEveryRow(t *testing.T) {
	res := testResource(nil)
	fin := &finishImporter{}
	res.NewImporter = func(context.Context, Opts) (Importer, error) { return fin, nil }
	reg := newTestRegistry(t, res)
	res2, err := runImport(t, reg, FormatCSV, "sku\nA\nB\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if fin.rows != 2 || fin.finishedAt != 2 {
		t.Fatalf("Finish must see every row: %+v", fin)
	}
	// Deferred errors blame their row even though the row itself landed, so Errors outnumbers Failed
	if len(res2.Errors) != 1 || res2.Errors[0].Line != 2 || res2.Failed != 0 {
		t.Fatalf("want the deferred error merged without counting as a failed row: %+v", res2)
	}
}

type finishImporter struct {
	rows       int
	finishedAt int
}

func (f *finishImporter) Row(context.Context, *Row) (Outcome, error) {
	f.rows++
	return OutcomeCreated, nil
}

func (f *finishImporter) Finish(context.Context) ([]RowError, error) {
	f.finishedAt = f.rows
	return []RowError{{Line: 2, Message: "deferred work failed"}}, nil
}

func TestExportStreamsDeclaredThenDynamicColumns(t *testing.T) {
	res := testResource(nil)
	res.NewImporter = nil
	res.NewExporter = func(context.Context, Opts) (Exporter, error) { return fakeExporter{}, nil }
	reg := newTestRegistry(t, res)

	var buf bytes.Buffer
	out, err := reg.Export(context.Background(), Run{Resource: "widgets", Format: FormatCSV}, &buf)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if out.Rows != 2 {
		t.Fatalf("want 2 rows, got %d", out.Rows)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if got := strings.TrimSpace(lines[0]); got != "sku,name,price,active,attr:color" {
		t.Fatalf("dynamic columns belong after the declared ones, got %q", got)
	}
	if !strings.HasPrefix(lines[1], "A,Widget A,1999,true,red") {
		t.Fatalf("unexpected first row %q", lines[1])
	}
}

type fakeExporter struct{}

func (fakeExporter) Columns(context.Context) ([]Field, error) {
	return []Field{{Name: "attr:color", Type: FieldText}}, nil
}

func (fakeExporter) Stream(_ context.Context, emit func(*Row) error) error {
	for _, sku := range []string{"A", "B"} {
		r := NewRow()
		r.Set("sku", sku)
		r.Set("name", "Widget "+sku)
		r.Set("price", int64(1999))
		r.Set("active", true)
		r.Set("attr:color", "red")
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// A CSV export must re-import cleanly, which is the whole point of sharing one column set
func TestCSVRoundTrip(t *testing.T) {
	res := testResource(nil)
	res.NewExporter = func(context.Context, Opts) (Exporter, error) { return fakeExporter{}, nil }
	var seen []string
	res.NewImporter = func(context.Context, Opts) (Importer, error) {
		return importerFunc(func(_ context.Context, r *Row) (Outcome, error) {
			seen = append(seen, r.Str("sku")+"/"+r.Str("attr:color")+"/"+r.Str("price"))
			return OutcomeUpdated, nil
		}), nil
	}
	reg := newTestRegistry(t, res)

	var buf bytes.Buffer
	if _, err := reg.Export(context.Background(), Run{Resource: "widgets", Format: FormatCSV}, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	out, err := runImport(t, reg, FormatCSV, buf.String())
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if out.Updated != 2 || out.Failed != 0 {
		t.Fatalf("round trip should update both rows: %+v", out)
	}
	if seen[0] != "A/red/1999" {
		t.Fatalf("values did not survive the round trip: %v", seen)
	}
}

func TestJSONLRoundTrip(t *testing.T) {
	res := testResource(nil)
	res.NewExporter = func(context.Context, Opts) (Exporter, error) { return fakeExporter{}, nil }
	var count int
	res.NewImporter = func(context.Context, Opts) (Importer, error) {
		return importerFunc(func(_ context.Context, r *Row) (Outcome, error) {
			count++
			if r.Str("name") == "" {
				return OutcomeSkipped, errors.New("name lost in transit")
			}
			return OutcomeCreated, nil
		}), nil
	}
	reg := newTestRegistry(t, res)

	var buf bytes.Buffer
	if _, err := reg.Export(context.Background(), Run{Resource: "widgets", Format: FormatJSONL}, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	out, err := runImport(t, reg, FormatJSONL, buf.String())
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if out.Created != 2 || count != 2 {
		t.Fatalf("jsonl round trip failed: %+v", out)
	}
}

func TestCSVReaderRejectsDuplicateColumns(t *testing.T) {
	reg := newTestRegistry(t, testResource(func(context.Context, *Row) (Outcome, error) { return OutcomeCreated, nil }))
	if _, err := runImport(t, reg, FormatCSV, "sku,sku\nA,B\n"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("want a duplicate-column rejection, got %v", err)
	}
}

func TestCSVReaderStripsBOMAndSkipsBlankRows(t *testing.T) {
	var seen int
	reg := newTestRegistry(t, testResource(func(context.Context, *Row) (Outcome, error) {
		seen++
		return OutcomeCreated, nil
	}))
	res, err := runImport(t, reg, FormatCSV, "\ufeffsku,name\nA,x\n\n,\nB,y\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if seen != 2 || res.Total != 2 {
		t.Fatalf("want the BOM stripped and blank rows dropped, got %d rows", res.Total)
	}
}

func TestTemplateCarriesHeaderAndExample(t *testing.T) {
	reg := newTestRegistry(t, testResource(func(context.Context, *Row) (Outcome, error) { return OutcomeCreated, nil }))
	var buf bytes.Buffer
	if err := reg.Template(context.Background(), "widgets", FormatCSV, &buf); err != nil {
		t.Fatalf("template: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if strings.TrimSpace(lines[0]) != "sku,name,price,active" {
		t.Fatalf("unexpected template header %q", lines[0])
	}
	if !strings.Contains(lines[1], "1999") {
		t.Fatalf("the example row should carry declared examples, got %q", lines[1])
	}
}

func TestResolveOptsAppliesDefaultsAndRejectsBadChoice(t *testing.T) {
	res := testResource(func(context.Context, *Row) (Outcome, error) { return OutcomeCreated, nil })
	res.Options = []OptionDef{
		{Name: "locale", Type: OptString, Default: "en"},
		{Name: "mode", Type: OptSelect, Choices: []Choice{{Value: "a"}, {Value: "b"}}},
		{Name: "needed", Type: OptString, Required: true, Applies: DirectionImport},
	}
	if _, err := resolveOpts(res, NewOpts(map[string]string{"needed": "x", "mode": "z"}), DirectionImport); err == nil {
		t.Fatal("want a rejection for an out-of-set choice")
	}
	if _, err := resolveOpts(res, NewOpts(nil), DirectionImport); err == nil {
		t.Fatal("want a rejection for the missing required option")
	}
	got, err := resolveOpts(res, NewOpts(map[string]string{"needed": "x", "unknown": "y"}), DirectionImport)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Str("locale") != "en" {
		t.Fatalf("default not applied: %q", got.Str("locale"))
	}
	if got.Str("unknown") != "" {
		t.Fatal("an undeclared option must not survive into the run")
	}
}

// An export-only option must not be demanded of an import run
func TestResolveOptsHonoursDirection(t *testing.T) {
	res := testResource(func(context.Context, *Row) (Outcome, error) { return OutcomeCreated, nil })
	res.Options = []OptionDef{{Name: "since", Type: OptDate, Required: true, Applies: DirectionExport}}
	if _, err := resolveOpts(res, NewOpts(nil), DirectionImport); err != nil {
		t.Fatalf("an export-only option leaked into the import run: %v", err)
	}
}

func TestRegisterRejectsImporterWithoutIdentifier(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("an importer with no upsert key must not register")
		}
	}()
	reg := NewRegistry()
	reg.Register(Resource{
		Slug: "x", Label: "X", ViewPermission: "a", ImportPermission: "b",
		Fields:      []Field{{Name: "name"}},
		NewImporter: func(context.Context, Opts) (Importer, error) { return nil, nil },
	})
}

func TestRegisterRejectsDuplicateSlug(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("two resources on one slug must not register")
		}
	}()
	reg := newTestRegistry(t, testResource(nil))
	reg.Register(testResource(nil))
}

func TestListIsStableAcrossRegistrationOrder(t *testing.T) {
	reg := NewRegistry()
	for _, slug := range []string{"zebra", "alpha", "mid"} {
		r := testResource(nil)
		r.Slug, r.Label = slug, slug
		reg.Register(r)
	}
	got := strings.Join(reg.Slugs(), ",")
	if got != "alpha,mid,zebra" {
		t.Fatalf("want group-then-label order, got %q", got)
	}
}

// A missing required option is knowable when the run is requested, so it must not become a doomed job
func TestValidateOptionsRejectsBeforeRunning(t *testing.T) {
	res := testResource(func(context.Context, *Row) (Outcome, error) { return OutcomeCreated, nil })
	res.Options = []OptionDef{{Name: "rule_id", Type: OptSelect, Required: true, Choices: []Choice{{Value: "x"}}}}
	reg := newTestRegistry(t, res)

	if err := reg.ValidateOptions("widgets", NewOpts(nil), DirectionExport); err == nil {
		t.Fatal("want the missing required option rejected up front")
	}
	if err := reg.ValidateOptions("widgets", NewOpts(map[string]string{"rule_id": "x"}), DirectionExport); err != nil {
		t.Fatalf("a satisfied option should pass: %v", err)
	}
	if err := reg.ValidateOptions("nope", NewOpts(nil), DirectionExport); err == nil {
		t.Fatal("want an unknown resource rejected")
	}
}

// The declared Enum is the only place the allowed set lives, so a resource can never drift from what the UI shows
func TestDriverRejectsValueOutsideDeclaredEnum(t *testing.T) {
	var reached bool
	res := testResource(func(context.Context, *Row) (Outcome, error) {
		reached = true
		return OutcomeCreated, nil
	})
	res.Fields = append(res.Fields, Field{Name: "kind", Type: FieldText, Enum: []string{"simple", "configurable"}})
	reg := newTestRegistry(t, res)

	out, err := runImport(t, reg, FormatCSV, "sku,kind\nA,bundle\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if out.Failed != 1 || reached {
		t.Fatalf("the row must fail before the handler sees it: failed=%d reached=%v", out.Failed, reached)
	}
	if !strings.Contains(out.Errors[0].Message, "simple, configurable") {
		t.Fatalf("the error should name the allowed set: %q", out.Errors[0].Message)
	}
}

func TestDeclaredEnumIsCaseInsensitiveAndSkipsBlanks(t *testing.T) {
	res := testResource(func(_ context.Context, r *Row) (Outcome, error) {
		if got := r.Enum("kind"); got != "" && got != "simple" {
			t.Errorf("Enum should hand back the lowered value, got %q", got)
		}
		return OutcomeCreated, nil
	})
	res.Fields = append(res.Fields, Field{Name: "kind", Type: FieldText, Enum: []string{"simple", "configurable"}})
	reg := newTestRegistry(t, res)

	out, err := runImport(t, reg, FormatCSV, "sku,kind\nA,SIMPLE\nB,\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if out.Created != 2 {
		t.Fatalf("a differently-cased value and a blank must both pass: %+v", out)
	}
}

// RequireDeclared reads the descriptor, so the required set can't drift from the list a resource passes by hand
func TestRequireDeclaredReadsTheDescriptor(t *testing.T) {
	res := testResource(func(_ context.Context, r *Row) (Outcome, error) {
		if !r.RequireDeclared() {
			return OutcomeSkipped, r.Err()
		}
		return OutcomeCreated, nil
	})
	reg := newTestRegistry(t, res)

	out, err := runImport(t, reg, FormatCSV, "sku,name\nA,ok\n,missing\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if out.Created != 1 || out.Failed != 1 {
		t.Fatalf("want the row missing a declared-required column failed: %+v", out)
	}
	if !strings.Contains(out.Errors[0].Message, "sku is required") {
		t.Fatalf("error should name the missing column: %q", out.Errors[0].Message)
	}
}

// Read-only columns are export context - demanding them on import would reject every hand-made file
func TestRequireDeclaredIgnoresReadOnlyColumns(t *testing.T) {
	res := testResource(func(_ context.Context, r *Row) (Outcome, error) {
		if !r.RequireDeclared() {
			return OutcomeSkipped, r.Err()
		}
		return OutcomeCreated, nil
	})
	res.Fields = append(res.Fields, Field{Name: "computed", Type: FieldText, Required: true, ReadOnly: true})
	reg := newTestRegistry(t, res)

	out, err := runImport(t, reg, FormatCSV, "sku\nA\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if out.Created != 1 {
		t.Fatalf("a read-only required column must not be demanded of an import: %+v", out)
	}
}

func TestRegisterRejectsSelectWithNothingToSelect(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a select with neither choices nor a source renders an empty dropdown nobody can satisfy")
		}
	}()
	res := testResource(func(context.Context, *Row) (Outcome, error) { return OutcomeCreated, nil })
	res.Options = []OptionDef{{Name: "rule_id", Type: OptSelect, Required: true}}
	newTestRegistry(t, res)
}

func TestChoicesPrefersTheRegisteredSource(t *testing.T) {
	reg := NewRegistry()
	reg.RegisterSource("rules", func(context.Context) ([]Choice, error) {
		return []Choice{{Value: "live", Label: "From the database"}}, nil
	})
	opt := OptionDef{Name: "rule_id", Type: OptSelect, Source: "rules", Choices: []Choice{{Value: "stale"}}}

	got, err := reg.Choices(context.Background(), opt)
	if err != nil {
		t.Fatalf("choices: %v", err)
	}
	if len(got) != 1 || got[0].Value != "live" {
		t.Fatalf("a Source must win over static Choices, got %+v", got)
	}
	if _, err := reg.Choices(context.Background(), OptionDef{Name: "x", Source: "nope"}); err == nil {
		t.Fatal("an unregistered source must error rather than silently yield nothing")
	}
}

// Validated after the providers register, so a typo fails the boot instead of rendering an empty dropdown
func TestValidateSourcesCatchesATypo(t *testing.T) {
	res := testResource(func(context.Context, *Row) (Outcome, error) { return OutcomeCreated, nil })
	res.Options = []OptionDef{{Name: "rule_id", Type: OptSelect, Source: "promotoin_rules"}}
	reg := newTestRegistry(t, res)

	if err := reg.ValidateSources(); err == nil {
		t.Fatal("want the misspelled source name rejected")
	}
	reg.RegisterSource("promotoin_rules", func(context.Context) ([]Choice, error) { return nil, nil })
	if err := reg.ValidateSources(); err != nil {
		t.Fatalf("a registered source should validate: %v", err)
	}
}

func TestImportRetriesDeferredRowsToAFixedPoint(t *testing.T) {
	landed := map[string]bool{}
	reg := newTestRegistry(t, testResource(func(_ context.Context, r *Row) (Outcome, error) {
		if dep := r.Str("name"); dep != "" && !landed[dep] {
			return OutcomeSkipped, Defer(errors.New("waiting on " + dep))
		}
		landed[r.Str("sku")] = true
		return OutcomeCreated, nil
	}))
	res, err := runImport(t, reg, FormatCSV, "sku,name\nC,B\nB,A\nA,\nX,Y\nY,X\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Created != 3 || res.Failed != 2 || len(res.Errors) != 2 || !strings.Contains(res.Errors[0].Message, "waiting on") {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestImportReadsAnAliasHeaderAsItsColumn(t *testing.T) {
	res := testResource(func(_ context.Context, r *Row) (Outcome, error) {
		if r.Str("name") != "legacy" {
			return OutcomeSkipped, errors.New("alias not mapped")
		}
		return OutcomeCreated, nil
	})
	res.Fields[1].Aliases = []string{"title"}
	out, err := runImport(t, newTestRegistry(t, res), FormatCSV, "sku,title\nA,legacy\n")
	if err != nil || out.Created != 1 || len(out.Warnings) != 0 {
		t.Fatalf("result %+v, err %v", out, err)
	}
}

func TestRegisterRejectsABlankOrRepeatedViewAlsoPermission(t *testing.T) {
	for name, also := range map[string][]string{"blank": {""}, "same as view": {"test.view"}, "repeated": {"x.view", "x.view"}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: registered, want a panic", name)
				}
			}()
			res := testResource(nil)
			res.ViewAlsoRequires = also
			NewRegistry().Register(res)
		}()
	}
	res := testResource(nil)
	res.ViewAlsoRequires = []string{"other.view"}
	NewRegistry().Register(res)
	if got := res.ViewPermissions(); len(got) != 2 || got[0] != "test.view" || got[1] != "other.view" {
		t.Fatalf("view permissions = %v", got)
	}
}

func TestAnOptionValidatorRefusesTheRunAsAnInvalidOption(t *testing.T) {
	res := testResource(nil)
	res.Options = []OptionDef{{Name: "since", Label: "Since", Type: OptDate, Validate: func(v string) error {
		if v != "ok" {
			return errors.New("bad")
		}
		return nil
	}}}
	reg := newTestRegistry(t, res)
	if err := reg.ValidateOptions("widgets", NewOpts(map[string]string{"since": "nope"}), DirectionImport); !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("err = %v, want ErrInvalidOption", err)
	}
	if err := reg.ValidateOptions("widgets", NewOpts(map[string]string{"since": "ok"}), DirectionImport); err != nil {
		t.Fatalf("a valid value was refused: %v", err)
	}
	if err := reg.ValidateOptions("widgets", NewOpts(nil), DirectionImport); err != nil {
		t.Fatalf("a blank optional value reached the validator: %v", err)
	}
}
