package dataio

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// One record in either direction. Readers accumulate cell errors instead of returning them, so a row
// reports every bad cell at once rather than one per re-upload
type Row struct {
	Line int
	Ref  string // natural key echoed in error messages, set by the importer once it knows it

	values  map[string]string
	present map[string]bool
	errs    []string
	// The resource's declared columns, attached by the import driver - what makes Enum and RequireDeclared
	// read the descriptor rather than a list repeated at the call site
	fields *fieldSet
}

// Declaration order is kept alongside the lookup map so a row missing several columns always reports them the same way
type fieldSet struct {
	byName map[string]Field
	order  []Field
}

func newFieldSet(fields []Field) *fieldSet {
	fs := &fieldSet{byName: make(map[string]Field, len(fields)), order: fields}
	for _, f := range fields {
		fs.byName[f.Name] = f
	}
	return fs
}

func NewRow() *Row {
	return &Row{values: map[string]string{}, present: map[string]bool{}}
}

func (r *Row) Reset() {
	r.Line, r.Ref = 0, ""
	r.errs = r.errs[:0]
	r.fields = nil
	clear(r.values)
	clear(r.present)
}

// Whether the column exists in the file at all, regardless of whether this row filled it - the difference between "clear this" and "leave it alone"
func (r *Row) Column(name string) bool { return r.present[name] }

// Every column on the row, sorted so a resource scanning for a prefix family gets a stable order
func (r *Row) Names() []string {
	out := make([]string, 0, len(r.present))
	for name := range r.present {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Columns sharing a prefix, e.g. the attr: family - the prefix is kept on the returned names
func (r *Row) WithPrefix(prefix string) []string {
	var out []string
	for _, name := range r.Names() {
		if strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	return out
}

// Whether this row carries a usable value - the common guard for partial updates
func (r *Row) Has(name string) bool { return strings.TrimSpace(r.values[name]) != "" }

func (r *Row) Raw(name string) string { return r.values[name] }

func (r *Row) Str(name string) string { return strings.TrimSpace(r.values[name]) }

func (r *Row) Lower(name string) string { return strings.ToLower(r.Str(name)) }

func (r *Row) Upper(name string) string { return strings.ToUpper(r.Str(name)) }

func (r *Row) Fail(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

func (r *Row) bad(name, raw, want string) {
	r.Fail("%s: %q is not %s", name, raw, want)
}

// Non-nil once any accessor rejected a cell - the driver checks it after the importer returns, so a handler that ignores a bad cell still fails the row
func (r *Row) Err() error {
	if len(r.errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(r.errs, "; "))
}

func (r *Row) Int(name string) int64 {
	v := r.OptInt(name)
	if v == nil {
		return 0
	}
	return *v
}

// nil for a blank cell, which callers treat as "leave unchanged"
func (r *Row) OptInt(name string) *int64 {
	raw := r.Str(name)
	if raw == "" {
		return nil
	}
	n, err := parseInt(raw)
	if err != nil {
		r.bad(name, raw, "a whole number")
		return nil
	}
	return &n
}

func (r *Row) Int32(name string) *int32 {
	v := r.OptInt(name)
	if v == nil {
		return nil
	}
	if *v > 2147483647 || *v < -2147483648 {
		r.bad(name, r.Str(name), "within 32-bit range")
		return nil
	}
	n := int32(*v)
	return &n
}

func (r *Row) Float(name string) float64 {
	v := r.OptFloat(name)
	if v == nil {
		return 0
	}
	return *v
}

func (r *Row) OptFloat(name string) *float64 {
	raw := r.Str(name)
	if raw == "" {
		return nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		r.bad(name, raw, "a number")
		return nil
	}
	return &f
}

func (r *Row) Bool(name string) bool {
	v := r.OptBool(name)
	return v != nil && *v
}

func (r *Row) OptBool(name string) *bool {
	raw := r.Str(name)
	if raw == "" {
		return nil
	}
	v, ok := parseBool(raw)
	if !ok {
		r.bad(name, raw, "a yes/no value")
		return nil
	}
	return &v
}

// Accepts RFC3339 or a bare date, which is what spreadsheets emit
func (r *Row) Time(name string) *time.Time {
	raw := r.Str(name)
	if raw == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t
		}
	}
	r.bad(name, raw, "a date")
	return nil
}

func (r *Row) UUID(name string) *uuid.UUID {
	raw := r.Str(name)
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		r.bad(name, raw, "an id")
		return nil
	}
	return &id
}

// Pipe-separated because commas belong to the CSV - blanks are dropped
func (r *Row) List(name string) []string {
	parts := strings.Split(r.values[name], "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// The lowered cell. The allowed set is declared on the Field and enforced by the driver before the handler
// runs, so by the time a resource reads this the value is already known good
func (r *Row) Enum(name string) string { return r.Lower(name) }

// Fails the row unless every column the resource declared Required carries a value. The resource decides WHEN
// to call this - only it knows whether a row is a create - while the descriptor stays the one place the set lives
func (r *Row) RequireDeclared() bool {
	if r.fields == nil {
		return true
	}
	ok := true
	for _, f := range r.fields.order {
		if !f.Required || f.ReadOnly {
			continue
		}
		if !r.Has(f.Name) {
			r.Fail("%s is required", f.Name)
			ok = false
		}
	}
	return ok
}

// Rejects any cell that falls outside its Field's declared Enum - one enforcement point, so the allowed set can
// never drift from what the UI and the template advertise
func (r *Row) validateEnums() {
	if r.fields == nil {
		return
	}
	for _, name := range r.Names() {
		f, declared := r.fields.byName[name]
		if !declared || len(f.Enum) == 0 {
			continue
		}
		raw := r.Str(name)
		if raw == "" {
			continue
		}
		if !slices.ContainsFunc(f.Enum, func(a string) bool { return strings.EqualFold(a, raw) }) {
			r.bad(name, raw, "one of "+strings.Join(f.Enum, ", "))
		}
	}
}

func (r *Row) JSON(name string, into any) bool {
	raw := r.Str(name)
	if raw == "" {
		return false
	}
	if err := json.Unmarshal([]byte(raw), into); err != nil {
		r.bad(name, raw, "valid JSON")
		return false
	}
	return true
}

// Fails the row unless every named column carries a value - the create-time guard resources call before they build an input
func (r *Row) Require(names ...string) bool {
	ok := true
	for _, n := range names {
		if !r.Has(n) {
			r.Fail("%s is required", n)
			ok = false
		}
	}
	return ok
}

// Set stringifies any scalar the exporters hand it, so a mapper reads `r.Set("price", p.DefaultPrice)` rather than threading strconv through every column
func (r *Row) Set(name string, value any) {
	r.present[name] = true
	r.values[name] = stringify(value)
}

func (r *Row) SetList(name string, values []string) {
	r.Set(name, strings.Join(values, "|"))
}

func stringify(value any) string {
	switch t := value.(type) {
	case nil:
		return ""
	case string:
		return t
	case *string:
		if t == nil {
			return ""
		}
		return *t
	case bool:
		return strconv.FormatBool(t)
	case *bool:
		if t == nil {
			return ""
		}
		return strconv.FormatBool(*t)
	case int:
		return strconv.Itoa(t)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case *int32:
		if t == nil {
			return ""
		}
		return strconv.FormatInt(int64(*t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case *int64:
		if t == nil {
			return ""
		}
		return strconv.FormatInt(*t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case *float64:
		if t == nil {
			return ""
		}
		return strconv.FormatFloat(*t, 'f', -1, 64)
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.Format(time.RFC3339)
	case *time.Time:
		if t == nil || t.IsZero() {
			return ""
		}
		return t.Format(time.RFC3339)
	case uuid.UUID:
		if t == uuid.Nil {
			return ""
		}
		return t.String()
	case json.Number:
		return t.String()
	case []string:
		return strings.Join(t, "|")
	default:
		return fmt.Sprintf("%v", t)
	}
}

func parseInt(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

// ok is false for a blank or unrecognised cell, which callers read as "leave unchanged"
func parseBool(s string) (val, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "t":
		return true, true
	case "0", "false", "no", "n", "f":
		return false, true
	default:
		return false, false
	}
}
