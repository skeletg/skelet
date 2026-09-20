// Package webform owns custom, admin-built forms placed on CMS pages through the `form` block - an ordered list of typed fields with optional per-field conditional visibility, plus form-level settings for the submit label, success or redirect, admin-notify and confirmation emails, flow trigger and captcha Storefront visitors submit, and submissions are stored and surfaced to admins Mirrors the internal/review layout - domain.go is types, permission registration and pure validators; repository.go is the Postgres Repository; service.go is business logic, tx and events - Field schema and settings persist as JSONB, matching blockkit Document and email FieldDef, rather than per-field rows Distinct from internal/form, which is Marketing's fixed-schema signup and email-capture popups - this is the general-purpose builder
package webform

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func init() {
	permission.RegisterGroup(permission.Group{Slug: "webforms", Name: "Forms", SortOrder: 78})
	permission.Register(permission.Definition{Group: "webforms", Slug: "webforms.view", Name: "View Forms", SortOrder: 1})
	permission.Register(permission.Definition{Group: "webforms", Slug: "webforms.manage", Name: "Manage Forms", SortOrder: 2})
	// Submissions can hold PII; gate them behind a separate grant
	permission.Register(permission.Definition{Group: "webforms", Slug: "webforms.submissions_view", Name: "View Form Submissions", SortOrder: 3})
}

var (
	ErrFormNotFound       = errors.New("form not found")
	ErrSlugTaken          = errors.New("a form with this slug already exists")
	ErrInvalidSchema      = errors.New("invalid form schema")
	ErrInvalidSubmission  = errors.New("submission failed validation")
	ErrSpamRejected       = errors.New("submission rejected")
	ErrFormInactive       = errors.New("form is not accepting submissions")
	ErrSubmissionNotFound = errors.New("submission not found")
)

func init() {
	apperr.RegisterSentinel(ErrFormNotFound, "NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrSubmissionNotFound, "NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrSlugTaken, "CONFLICT", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrInvalidSchema, "VALIDATION", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidSubmission, "VALIDATION", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrSpamRejected, "VALIDATION", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrFormInactive, "VALIDATION", apperr.ClassValidation)
}

// Every field kind a form can contain, including the non-input layout elements - heading, paragraph, divider
type FieldType string

const (
	FieldText          FieldType = "text"
	FieldTextarea      FieldType = "textarea"
	FieldEmail         FieldType = "email"
	FieldNumber        FieldType = "number"
	FieldTel           FieldType = "tel"
	FieldURL           FieldType = "url"
	FieldDate          FieldType = "date"
	FieldDatetime      FieldType = "datetime"
	FieldTime          FieldType = "time"
	FieldSelect        FieldType = "select"         // single choice (dropdown)
	FieldRadio         FieldType = "radio"          // single choice (radios)
	FieldCheckboxGroup FieldType = "checkbox_group" // multi choice
	FieldConsent       FieldType = "consent"        // single boolean checkbox (opt-in)
	FieldFile          FieldType = "file"
	FieldHidden        FieldType = "hidden"
	FieldHeading       FieldType = "heading"   // layout
	FieldParagraph     FieldType = "paragraph" // layout
	FieldDivider       FieldType = "divider"   // layout
	// A group marker carrying no value - it visually groups the fields that follow, until the next section, and can hide the whole group through its own VisibleIf
	FieldSection FieldType = "section"
	// A grid with one question per Row and one or many choices per row drawn from Columns - the value is {rowKey: colValue} or {rowKey: [colValues]}
	FieldMatrix FieldType = "matrix"
	// A repeatable group of SubFields the visitor can add N times - the value is an array of answer objects, one per instance
	FieldRepeater FieldType = "repeater"
)

func (t FieldType) IsValid() bool {
	switch t {
	case FieldText, FieldTextarea, FieldEmail, FieldNumber, FieldTel, FieldURL,
		FieldDate, FieldDatetime, FieldTime, FieldSelect, FieldRadio,
		FieldCheckboxGroup, FieldConsent, FieldFile, FieldHidden,
		FieldHeading, FieldParagraph, FieldDivider,
		FieldSection, FieldMatrix, FieldRepeater:
		return true
	}
	return false
}

// A non-input decorative field that never collects a value, is never required and is never validated - sections are group markers gating following fields, so they don't count as layout here
func (t FieldType) IsLayout() bool {
	switch t {
	case FieldHeading, FieldParagraph, FieldDivider:
		return true
	}
	return false
}

// Whether a field type may appear inside a repeater - composite and layout types are disallowed to bound nesting depth
func subFieldAllowed(t FieldType) bool {
	switch t {
	case FieldSection, FieldMatrix, FieldRepeater, FieldFile,
		FieldHeading, FieldParagraph, FieldDivider:
		return false
	}
	return t.IsValid()
}

func (t FieldType) IsChoice() bool {
	switch t {
	case FieldSelect, FieldRadio, FieldCheckboxGroup:
		return true
	}
	return false
}

func (t FieldType) IsMulti() bool { return t == FieldCheckboxGroup }

type Op string

const (
	OpEquals    Op = "equals"
	OpNotEquals Op = "notEquals"
	OpContains  Op = "contains"
	OpOneOf     Op = "oneOf"
	OpEmpty     Op = "empty"
	OpNotEmpty  Op = "notEmpty"
	OpGt        Op = "gt"
	OpLt        Op = "lt"
)

func (o Op) IsValid() bool {
	switch o {
	case OpEquals, OpNotEquals, OpContains, OpOneOf, OpEmpty, OpNotEmpty, OpGt, OpLt:
		return true
	}
	return false
}

type Rule struct {
	Field  string   `json:"field"`
	Op     Op       `json:"op"`
	Value  string   `json:"value,omitempty"`
	Values []string `json:"values,omitempty"`
}

// ConditionGroup is a boolean tree of rules - A field is visible when its
// (optional) group evaluates true; a nil/empty group means always visible
type ConditionGroup struct {
	Logic  string           `json:"logic"` // "and" | "or"
	Rules  []Rule           `json:"rules,omitempty"`
	Groups []ConditionGroup `json:"groups,omitempty"`
}

type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type MatrixRow struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Validation struct {
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Pattern   string   `json:"pattern,omitempty"` // RE2 regex
}

// Field is one form field - Key is the stable identifier used in the answers
// map and referenced by conditional rules
type Field struct {
	Key          string          `json:"key"`
	Type         FieldType       `json:"type"`
	Label        string          `json:"label"`
	Placeholder  string          `json:"placeholder,omitempty"`
	HelpText     string          `json:"helpText,omitempty"`
	Required     bool            `json:"required,omitempty"`
	Options      []FieldOption   `json:"options,omitempty"`
	Validation   *Validation     `json:"validation,omitempty"`
	VisibleIf    *ConditionGroup `json:"visibleIf,omitempty"`
	DefaultValue string          `json:"defaultValue,omitempty"`
	MaxFileBytes int64           `json:"maxFileBytes,omitempty"`
	AcceptMimes  []string        `json:"acceptMimes,omitempty"`
	Boxed        bool            `json:"boxed,omitempty"`
	// Matrix config (matrix fields only): one question per row, choices from
	// columns; Multi lets a row pick more than one column
	Rows    []MatrixRow   `json:"rows,omitempty"`
	Columns []FieldOption `json:"columns,omitempty"`
	Multi   bool          `json:"multi,omitempty"`
	// Repeater config (repeater fields only): the per-instance sub-fields and the
	// allowed instance-count bounds
	SubFields []Field `json:"subFields,omitempty"`
	MinItems  *int    `json:"minItems,omitempty"`
	MaxItems  *int    `json:"maxItems,omitempty"`
}

type ConfirmationEmail struct {
	Enabled    bool   `json:"enabled,omitempty"`
	ToFieldKey string `json:"toFieldKey,omitempty"` // which email field to send to
	Subject    string `json:"subject,omitempty"`
	Body       string `json:"body,omitempty"` // plain text
}

type FormSettings struct {
	SubmitLabel    string            `json:"submitLabel,omitempty"`
	SuccessMessage string            `json:"successMessage,omitempty"`
	RedirectURL    string            `json:"redirectUrl,omitempty"`
	NotifyEmails   []string          `json:"notifyEmails,omitempty"` // admin recipients (never sent to storefront)
	Confirmation   ConfirmationEmail `json:"confirmation"`
	ConsentKind    string            `json:"consentKind,omitempty"` // record marketing consent under this kind
	FlowTrigger    bool              `json:"flowTrigger,omitempty"` // fire the webform_submitted flow trigger
	RequireCaptcha bool              `json:"requireCaptcha,omitempty"`
}

type Status string

const (
	StatusDraft  Status = "draft"
	StatusActive Status = "active"
)

func (s Status) IsValid() bool { return s == StatusDraft || s == StatusActive }

type Form struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Slug      string
	Name      string
	Status    Status
	Fields    []Field
	Settings  FormSettings
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type SubmissionFile struct {
	FieldKey  string `json:"fieldKey"`
	ObjectKey string `json:"objectKey"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	MimeType  string `json:"mimeType"`
}

type SubmissionMeta struct {
	IP       string `json:"ip,omitempty"`
	UA       string `json:"ua,omitempty"`
	Referer  string `json:"referer,omitempty"`
	PagePath string `json:"pagePath,omitempty"`
	Locale   string `json:"locale,omitempty"`
}

type Submission struct {
	ID         uuid.UUID
	FormID     uuid.UUID
	ChannelID  uuid.UUID
	Answers    map[string]any
	Files      []SubmissionFile
	CustomerID *uuid.UUID
	Meta       SubmissionMeta
	CreatedAt  time.Time
}

type FieldError struct {
	Field   string
	Message string
}

// SubmissionError bundles field-level errors and unwraps to ErrInvalidSubmission
// so apperr classifies it as VALIDATION
type SubmissionError struct {
	Fields []FieldError
}

func (e *SubmissionError) Error() string { return ErrInvalidSubmission.Error() }
func (e *SubmissionError) Unwrap() error { return ErrInvalidSubmission }

type SaveInput struct {
	Slug     string
	Name     string
	Status   Status
	Fields   []Field
	Settings FormSettings
}

var fieldKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ValidateSchema checks a form's field list is well-formed: valid, unique, well-named keys; valid types; options
// present for choice fields; valid condition operators referencing existing keys; compilable regex patterns
func ValidateSchema(fields []Field) error {
	// Value-bearing keys, for resolving VisibleIf references - Layout + section
	// markers carry no value and are excluded
	keys := make(map[string]bool, len(fields))
	for _, f := range fields {
		if f.Type.IsLayout() || f.Type == FieldSection {
			continue
		}
		keys[f.Key] = true
	}
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		if !f.Type.IsValid() {
			return errors.Join(ErrInvalidSchema, errorsf("field %q has invalid type %q", f.Key, f.Type))
		}
		if f.Type.IsLayout() {
			continue
		}
		// Sections gate following fields; validate only their condition
		if f.Type == FieldSection {
			if err := validateGroup(f.VisibleIf, keys, "section"); err != nil {
				return err
			}
			continue
		}
		if !fieldKeyRe.MatchString(f.Key) {
			return errors.Join(ErrInvalidSchema, errorsf("field key %q must match ^[a-z][a-z0-9_]*$", f.Key))
		}
		if seen[f.Key] {
			return errors.Join(ErrInvalidSchema, errorsf("duplicate field key %q", f.Key))
		}
		seen[f.Key] = true
		if f.Type.IsChoice() && len(f.Options) == 0 {
			return errors.Join(ErrInvalidSchema, errorsf("field %q is a choice type but has no options", f.Key))
		}
		if f.Validation != nil && f.Validation.Pattern != "" {
			if _, err := regexp.Compile(f.Validation.Pattern); err != nil {
				return errors.Join(ErrInvalidSchema, errorsf("field %q has an invalid pattern: %v", f.Key, err))
			}
		}
		if err := validateMatrixRepeaterSchema(f); err != nil {
			return err
		}
		if err := validateGroup(f.VisibleIf, keys, f.Key); err != nil {
			return err
		}
	}
	return nil
}

func validateMatrixRepeaterSchema(f Field) error {
	switch f.Type {
	case FieldMatrix:
		if len(f.Rows) == 0 || len(f.Columns) == 0 {
			return errors.Join(ErrInvalidSchema, errorsf("matrix field %q needs at least one row and one column", f.Key))
		}
		rowKeys := map[string]bool{}
		for _, r := range f.Rows {
			if !fieldKeyRe.MatchString(r.Key) {
				return errors.Join(ErrInvalidSchema, errorsf("matrix %q row key %q must match ^[a-z][a-z0-9_]*$", f.Key, r.Key))
			}
			if rowKeys[r.Key] {
				return errors.Join(ErrInvalidSchema, errorsf("matrix %q has duplicate row key %q", f.Key, r.Key))
			}
			rowKeys[r.Key] = true
		}
		colVals := map[string]bool{}
		for _, c := range f.Columns {
			if c.Value == "" {
				return errors.Join(ErrInvalidSchema, errorsf("matrix %q has a column with no value", f.Key))
			}
			if colVals[c.Value] {
				return errors.Join(ErrInvalidSchema, errorsf("matrix %q has duplicate column value %q", f.Key, c.Value))
			}
			colVals[c.Value] = true
		}
	case FieldRepeater:
		if len(f.SubFields) == 0 {
			return errors.Join(ErrInvalidSchema, errorsf("repeater field %q needs at least one sub-field", f.Key))
		}
		if f.MinItems != nil && f.MaxItems != nil && *f.MaxItems < *f.MinItems {
			return errors.Join(ErrInvalidSchema, errorsf("repeater %q maxItems < minItems", f.Key))
		}
		subKeys := map[string]bool{}
		subKeySet := map[string]bool{}
		for _, s := range f.SubFields {
			if !subFieldAllowed(s.Type) {
				return errors.Join(ErrInvalidSchema, errorsf("repeater %q sub-field %q has an unsupported type %q", f.Key, s.Key, s.Type))
			}
			subKeySet[s.Key] = true
		}
		for _, s := range f.SubFields {
			if !fieldKeyRe.MatchString(s.Key) {
				return errors.Join(ErrInvalidSchema, errorsf("repeater %q sub-field key %q must match ^[a-z][a-z0-9_]*$", f.Key, s.Key))
			}
			if subKeys[s.Key] {
				return errors.Join(ErrInvalidSchema, errorsf("repeater %q has duplicate sub-field key %q", f.Key, s.Key))
			}
			subKeys[s.Key] = true
			if s.Type.IsChoice() && len(s.Options) == 0 {
				return errors.Join(ErrInvalidSchema, errorsf("repeater %q sub-field %q is a choice type but has no options", f.Key, s.Key))
			}
			if s.Validation != nil && s.Validation.Pattern != "" {
				if _, err := regexp.Compile(s.Validation.Pattern); err != nil {
					return errors.Join(ErrInvalidSchema, errorsf("repeater %q sub-field %q has an invalid pattern: %v", f.Key, s.Key, err))
				}
			}
			if err := validateGroup(s.VisibleIf, subKeySet, s.Key); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateGroup(g *ConditionGroup, keys map[string]bool, owner string) error {
	if g == nil {
		return nil
	}
	if g.Logic != "" && g.Logic != "and" && g.Logic != "or" {
		return errors.Join(ErrInvalidSchema, errorsf("field %q has an invalid condition logic %q", owner, g.Logic))
	}
	for _, r := range g.Rules {
		if !r.Op.IsValid() {
			return errors.Join(ErrInvalidSchema, errorsf("field %q references an invalid operator %q", owner, r.Op))
		}
		if r.Field == owner {
			return errors.Join(ErrInvalidSchema, errorsf("field %q cannot depend on itself", owner))
		}
		if !keys[r.Field] {
			return errors.Join(ErrInvalidSchema, errorsf("field %q condition references unknown field %q", owner, r.Field))
		}
	}
	for i := range g.Groups {
		if err := validateGroup(&g.Groups[i], keys, owner); err != nil {
			return err
		}
	}
	return nil
}

// VisibleFields returns the subset of value-bearing fields that are visible given the current answers, evaluating each field's condition tree AND the gating of the section it belongs to (a hidden section hides every field under it until the next section) - Layout + section markers are excluded from the result - This is the authoritative server-side view - hidden fields are neither required nor accepted
func VisibleFields(fields []Field, answers map[string]any) []Field {
	out := make([]Field, 0, len(fields))
	sectionVisible := true // fields before the first section are always in scope
	for _, f := range fields {
		if f.Type == FieldSection {
			sectionVisible = evalGroup(f.VisibleIf, answers)
			continue
		}
		if f.Type.IsLayout() {
			continue
		}
		if sectionVisible && evalGroup(f.VisibleIf, answers) {
			out = append(out, f)
		}
	}
	return out
}

func evalGroup(g *ConditionGroup, answers map[string]any) bool {
	if g == nil || (len(g.Rules) == 0 && len(g.Groups) == 0) {
		return true // no condition → always visible
	}
	or := g.Logic == "or"
	results := make([]bool, 0, len(g.Rules)+len(g.Groups))
	for _, r := range g.Rules {
		results = append(results, evalRule(r, answers))
	}
	for i := range g.Groups {
		results = append(results, evalGroup(&g.Groups[i], answers))
	}
	if or {
		for _, ok := range results {
			if ok {
				return true
			}
		}
		return false
	}
	for _, ok := range results {
		if !ok {
			return false
		}
	}
	return true
}

func evalRule(r Rule, answers map[string]any) bool {
	got := answers[r.Field]
	switch r.Op {
	case OpEmpty:
		return isEmptyAnswer(got)
	case OpNotEmpty:
		return !isEmptyAnswer(got)
	case OpEquals:
		return anyContainsString(got, r.Value, true)
	case OpNotEquals:
		return !anyContainsString(got, r.Value, true)
	case OpContains:
		return anyContainsString(got, r.Value, false)
	case OpOneOf:
		for _, v := range r.Values {
			if anyContainsString(got, v, true) {
				return true
			}
		}
		return false
	case OpGt, OpLt:
		a, ok1 := toFloat(got)
		b, ok2 := toFloat(r.Value)
		if !ok1 || !ok2 {
			return false
		}
		if r.Op == OpGt {
			return a > b
		}
		return a < b
	}
	return false
}

func isEmptyAnswer(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case bool:
		return !t
	case []any:
		return len(t) == 0
	case []string:
		return len(t) == 0
	}
	return false
}

// anyContainsString compares an answer (scalar or list) against target - When exact is true it matches whole values
// (equals/oneOf); otherwise substring (contains) - List answers match if any element matches
func anyContainsString(v any, target string, exact bool) bool {
	match := func(s string) bool {
		if exact {
			return s == target
		}
		return strings.Contains(s, target)
	}
	switch t := v.(type) {
	case string:
		return match(t)
	case bool:
		return match(strconv.FormatBool(t))
	case float64:
		return match(strconv.FormatFloat(t, 'f', -1, 64))
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && match(s) {
				return true
			}
		}
	case []string:
		if slices.ContainsFunc(t, match) {
			return true
		}
	}
	return false
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

func errorsf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
