// Package dataio is the one seam every bulk import and export goes through - a resource declares its columns plus a per-row upsert, and the framework owns parsing, validation, dry runs, error isolation and progress
// Adding a column is one Field entry and one line in the mapper: it reaches the export header, the import template, the admin UI's column reference and the docs without touching any of them
package dataio

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
)

type Direction string

const (
	DirectionImport Direction = "import"
	DirectionExport Direction = "export"
)

// How a column is parsed and rendered - the admin UI keys its input widget and its validation hint off this too
type FieldType string

const (
	FieldText    FieldType = "text"
	FieldInteger FieldType = "integer"
	FieldNumber  FieldType = "number"
	FieldBool    FieldType = "bool"
	FieldDate    FieldType = "date"
	FieldMoney   FieldType = "money" // integer minor units, never a decimal - float cents are how rounding bugs get in
	FieldList    FieldType = "list"  // pipe-separated, since commas belong to the CSV
	FieldJSON    FieldType = "json"
)

type Field struct {
	Name        string // the column header, snake_case
	Label       string
	Type        FieldType
	Description string
	Example     string
	// Required to CREATE a record - an update may leave it out, which is why the driver can't enforce it alone
	Required bool
	// Part of the natural key rows upsert on - the driver hard-fails a file missing one of these
	Identifier bool
	ReadOnly   bool     // exported for context, ignored on import
	Enum       []string // allowed values, validated by Row.Enum and listed in the UI
	Aliases    []string // older header names an import still accepts, read as this column
}

// Falls back to the column name so a resource can skip Label on self-explanatory columns
func (f Field) Title() string {
	if f.Label != "" {
		return f.Label
	}
	return f.Name
}

type Choice struct {
	Value string
	Label string
}

type OptionType string

const (
	OptString  OptionType = "string"
	OptInt     OptionType = "int"
	OptBool    OptionType = "bool"
	OptSelect  OptionType = "select"
	OptChannel OptionType = "channel" // resolved and permission-checked by the caller, never trusted from the payload
	OptLocale  OptionType = "locale"
	OptDate    OptionType = "date"
)

// A per-run knob - the admin UI renders the form straight off these, so a new option needs no frontend work
type OptionDef struct {
	Name        string
	Label       string
	Description string
	Type        OptionType
	Required    bool
	Default     string
	Choices     []Choice
	// Name of a registered ChoiceProvider, for choices the registry can't know statically - a promotion rule,
	// a segment. Takes priority over Choices, and is what keeps entity pickers out of the frontend
	Source  string
	Applies Direction // empty means both
	// Refuses a malformed value before the run is queued - the error reaches the operator wrapped in ErrInvalidOption
	Validate func(v string) error
}

// Loads an option's choices at request time - register by name once the services it reads through exist
type ChoiceProvider func(ctx context.Context) ([]Choice, error)

type Outcome int

const (
	OutcomeSkipped Outcome = iota
	OutcomeCreated
	OutcomeUpdated
	OutcomeDeleted
)

// Applies one decoded row - a returned error fails just that row, so the rest of the file still lands
type Importer interface {
	Row(ctx context.Context, r *Row) (Outcome, error)
}

// Optional on an Importer: rows are grouped by pass and every pass runs to completion before the next starts, which is how configurable parents get created before their variant children
type Passer interface {
	Pass(r *Row) int
}

// Optional on an Importer: runs once after the last pass, for work that needs every row to have landed first
// Row errors returned here are merged into the result so deferred work still blames the row that caused it, which is why Errors can outnumber Failed
type Finisher interface {
	Finish(ctx context.Context) ([]RowError, error)
}

// Columns is always called before Stream, so an exporter that must load to know its column set may load once and reuse
type Exporter interface {
	// Columns beyond the resource's declared Fields, resolved per run - product attribute columns depend on what the catalog actually holds
	Columns(ctx context.Context) ([]Field, error)
	Stream(ctx context.Context, emit func(*Row) error) error
}

// One importable/exportable thing. Registered at wiring time rather than from init() because the handlers close over live services
type Resource struct {
	Slug        string
	Label       string
	Group       string
	Description string

	ViewPermission string // gates export
	// Further grants an export needs on top of ViewPermission, held over the same channel scope
	ViewAlsoRequires []string
	ImportPermission string // gates import - empty means the resource is export-only
	// Set when a run's blast radius is the whole installation, so a channel-scoped grant must not be enough
	RequireGlobalPermission bool
	// Set when importing rewrites rows every channel shares, while an export stays channel-scoped
	RequireGlobalImport bool

	Fields  []Field
	Options []OptionDef

	NewExporter func(ctx context.Context, o Opts) (Exporter, error)
	NewImporter func(ctx context.Context, o Opts) (Importer, error)

	// Rejects a file bigger than this before any row runs - 0 uses DefaultMaxImportRows
	MaxImportRows int
	// Columns the file may carry that aren't declared Fields, e.g. the attr: prefix - unknown columns outside these are reported as warnings
	DynamicPrefixes []string
}

func (r Resource) GlobalFor(d Direction) bool {
	return r.RequireGlobalPermission || (d == DirectionImport && r.RequireGlobalImport)
}

func (r Resource) ViewPermissions() []string {
	return append([]string{r.ViewPermission}, r.ViewAlsoRequires...)
}

func (r Resource) CanImport() bool { return r.NewImporter != nil && r.ImportPermission != "" }
func (r Resource) CanExport() bool { return r.NewExporter != nil }

func (r Resource) field(name string) (Field, bool) {
	for _, f := range r.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

func (r Resource) identifiers() []string {
	var out []string
	for _, f := range r.Fields {
		if f.Identifier {
			out = append(out, f.Name)
		}
	}
	return out
}

func (r Resource) canonical(name string) string {
	for _, f := range r.Fields {
		if slices.Contains(f.Aliases, name) {
			return f.Name
		}
	}
	return name
}

func (r Resource) knownColumn(name string) bool {
	if _, ok := r.field(name); ok {
		return true
	}
	for _, p := range r.DynamicPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func (r Resource) option(name string) (OptionDef, bool) {
	for _, o := range r.Options {
		if o.Name == name {
			return o, true
		}
	}
	return OptionDef{}, false
}

// One run's resolved inputs. ChannelID and Locale are promoted out of the raw values because the caller resolves and permission-checks them before anything here runs
type Opts struct {
	values    map[string]string
	ChannelID uuid.UUID
	Locale    string
	DryRun    bool
	ActorID   uuid.UUID
}

func NewOpts(values map[string]string) Opts {
	if values == nil {
		values = map[string]string{}
	}
	return Opts{values: values}
}

func (o Opts) Values() map[string]string {
	out := make(map[string]string, len(o.values))
	maps.Copy(out, o.values)
	return out
}

func (o Opts) Str(name string) string { return strings.TrimSpace(o.values[name]) }

func (o Opts) Bool(name string) bool {
	v, _ := parseBool(o.Str(name))
	return v
}

func (o Opts) Int(name string) int64 {
	n, _ := parseInt(o.Str(name))
	return n
}

// A run request. Progress is called as rows land so a long job can report movement, and may be nil
type Run struct {
	Resource string
	Format   Format
	Opts     Opts
	Progress func(processed, total int)
}

type RowError struct {
	Line    int    `json:"line"`
	Ref     string `json:"ref"`
	Message string `json:"message"`
}

type ImportResult struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"` // rows that did not apply - deferred Finisher errors land in Errors without counting here
	// Can outnumber Failed: a row that itself landed can still have its deferred work fail
	Errors    []RowError `json:"errors"`
	Warnings  []string   `json:"warnings"`
	Truncated bool       `json:"truncated"` // more errors than MaxRowErrors, so Errors holds only the first batch
}

type ExportResult struct {
	Rows    int      `json:"rows"`
	Columns []string `json:"columns"`
}

var (
	ErrUnknownResource = fmt.Errorf("unknown data resource")
	ErrUnknownFormat   = fmt.Errorf("unknown data format")
	ErrNotImportable   = fmt.Errorf("resource does not support import")
	ErrNotExportable   = fmt.Errorf("resource does not support export")
	ErrInvalidOption   = fmt.Errorf("invalid option")
)

// Wraps a row error that another row of the file may resolve - retried until a round lands nothing, then failed with the cause
func Defer(cause error) error { return deferredError{cause: cause} }

type deferredError struct{ cause error }

func (e deferredError) Error() string { return e.cause.Error() }
func (e deferredError) Unwrap() error { return e.cause }

func isDeferred(err error) bool {
	var d deferredError
	return errors.As(err, &d)
}

// Rows past this are refused before anything runs - a 5M-row paste is a mistake, not a workload
const DefaultMaxImportRows = 250_000

// Past this the file is wrong, not the rows, and holding 200k messages helps nobody
const MaxRowErrors = 1000
