package webform

import "testing"

func ptrInt(i int) *int { return &i }

func TestValidateSchema(t *testing.T) {
	cases := []struct {
		name    string
		fields  []Field
		wantErr bool
	}{
		{"valid", []Field{{Key: "name", Type: FieldText, Label: "Name"}, {Key: "color", Type: FieldSelect, Options: []FieldOption{{Value: "r", Label: "Red"}}}}, false},
		{"bad key", []Field{{Key: "Name", Type: FieldText}}, true},
		{"duplicate key", []Field{{Key: "a", Type: FieldText}, {Key: "a", Type: FieldText}}, true},
		{"choice without options", []Field{{Key: "c", Type: FieldSelect}}, true},
		{"bad regex", []Field{{Key: "a", Type: FieldText, Validation: &Validation{Pattern: "("}}}, true},
		{"layout needs no key", []Field{{Type: FieldDivider}, {Key: "a", Type: FieldText}}, false},
		{"self-referential condition", []Field{{Key: "a", Type: FieldText, VisibleIf: &ConditionGroup{Rules: []Rule{{Field: "a", Op: OpEquals, Value: "x"}}}}}, true},
		{"unknown field condition", []Field{{Key: "a", Type: FieldText, VisibleIf: &ConditionGroup{Rules: []Rule{{Field: "ghost", Op: OpEquals, Value: "x"}}}}}, true},
		{"bad operator", []Field{{Key: "a", Type: FieldText}, {Key: "b", Type: FieldText, VisibleIf: &ConditionGroup{Rules: []Rule{{Field: "a", Op: "bogus", Value: "x"}}}}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSchema(tc.fields)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateSchema err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestVisibleFields_Conditional(t *testing.T) {
	fields := []Field{
		{Key: "contact_method", Type: FieldSelect, Options: []FieldOption{{Value: "email"}, {Value: "phone"}}},
		{Key: "email", Type: FieldEmail, Required: true, VisibleIf: &ConditionGroup{Rules: []Rule{{Field: "contact_method", Op: OpEquals, Value: "email"}}}},
		{Key: "phone", Type: FieldTel, Required: true, VisibleIf: &ConditionGroup{Rules: []Rule{{Field: "contact_method", Op: OpEquals, Value: "phone"}}}},
	}
	// contact_method=email → only email visible
	vis := VisibleFields(fields, map[string]any{"contact_method": "email"})
	if !hasField(vis, "email") || hasField(vis, "phone") {
		t.Fatalf("expected only email visible, got %v", keysOf(vis))
	}
	// contact_method=phone → only phone visible
	vis = VisibleFields(fields, map[string]any{"contact_method": "phone"})
	if !hasField(vis, "phone") || hasField(vis, "email") {
		t.Fatalf("expected only phone visible, got %v", keysOf(vis))
	}
}

func TestVisibleFields_AndOrGroups(t *testing.T) {
	// visible when (a == "1" AND b == "2") OR c notEmpty
	cond := &ConditionGroup{
		Logic: "or",
		Groups: []ConditionGroup{
			{Logic: "and", Rules: []Rule{{Field: "a", Op: OpEquals, Value: "1"}, {Field: "b", Op: OpEquals, Value: "2"}}},
		},
		Rules: []Rule{{Field: "c", Op: OpNotEmpty}},
	}
	fields := []Field{
		{Key: "a", Type: FieldText}, {Key: "b", Type: FieldText}, {Key: "c", Type: FieldText},
		{Key: "target", Type: FieldText, VisibleIf: cond},
	}
	if hasField(VisibleFields(fields, map[string]any{"a": "1"}), "target") {
		t.Fatal("target should be hidden when only a=1")
	}
	if !hasField(VisibleFields(fields, map[string]any{"a": "1", "b": "2"}), "target") {
		t.Fatal("target should be visible when a=1 AND b=2")
	}
	if !hasField(VisibleFields(fields, map[string]any{"c": "x"}), "target") {
		t.Fatal("target should be visible when c is not empty")
	}
}

// A required field hidden by its condition must NOT be enforced - the security
// property that a client can't be blocked (or a hidden field forced) by lying
func TestValidateAnswers_HiddenRequiredNotEnforced(t *testing.T) {
	fields := []Field{
		{Key: "wants_call", Type: FieldConsent},
		{Key: "phone", Type: FieldTel, Required: true, VisibleIf: &ConditionGroup{Rules: []Rule{{Field: "wants_call", Op: OpEquals, Value: "true"}}}},
	}
	// wants_call not set → phone hidden → no error for missing phone
	vis := VisibleFields(fields, map[string]any{})
	if errs := validateAnswers(vis, map[string]any{}); len(errs) != 0 {
		t.Fatalf("hidden required field should not error, got %v", errs)
	}
	// wants_call=true → phone visible + required → error when missing
	vis = VisibleFields(fields, map[string]any{"wants_call": true})
	if errs := validateAnswers(vis, map[string]any{"wants_call": true}); len(errs) == 0 {
		t.Fatal("visible required phone should error when missing")
	}
}

func TestValidateAnswers_TypesAndConstraints(t *testing.T) {
	fields := []Field{
		{Key: "email", Type: FieldEmail, Required: true},
		{Key: "age", Type: FieldNumber, Validation: &Validation{Min: f64(18), Max: f64(120)}},
		{Key: "bio", Type: FieldText, Validation: &Validation{MaxLength: ptrInt(5)}},
		{Key: "color", Type: FieldSelect, Options: []FieldOption{{Value: "r"}, {Value: "g"}}},
		{Key: "terms", Type: FieldConsent, Required: true},
	}
	vis := VisibleFields(fields, map[string]any{})
	errs := validateAnswers(vis, map[string]any{
		"email": "not-an-email",
		"age":   float64(9),
		"bio":   "way too long",
		"color": "purple",
		"terms": false,
	})
	for _, want := range []string{"email", "age", "bio", "color", "terms"} {
		if !hasErr(errs, want) {
			t.Errorf("expected error for %q, got %v", want, errs)
		}
	}

	// All valid → no errors
	errs = validateAnswers(vis, map[string]any{
		"email": "a@b.com", "age": float64(30), "bio": "hi", "color": "r", "terms": true,
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateSchema_Composite(t *testing.T) {
	cases := []struct {
		name    string
		fields  []Field
		wantErr bool
	}{
		{"matrix ok", []Field{{Key: "m", Type: FieldMatrix, Rows: []MatrixRow{{Key: "r1", Label: "Q1"}}, Columns: []FieldOption{{Value: "a", Label: "A"}}}}, false},
		{"matrix no rows", []Field{{Key: "m", Type: FieldMatrix, Columns: []FieldOption{{Value: "a"}}}}, true},
		{"matrix dup row key", []Field{{Key: "m", Type: FieldMatrix, Rows: []MatrixRow{{Key: "r1"}, {Key: "r1"}}, Columns: []FieldOption{{Value: "a"}}}}, true},
		{"repeater ok", []Field{{Key: "rep", Type: FieldRepeater, SubFields: []Field{{Key: "nm", Type: FieldText, Label: "Name"}}}}, false},
		{"repeater no subfields", []Field{{Key: "rep", Type: FieldRepeater}}, true},
		{"repeater disallows file subfield", []Field{{Key: "rep", Type: FieldRepeater, SubFields: []Field{{Key: "doc", Type: FieldFile}}}}, true},
		{"repeater disallows nested repeater", []Field{{Key: "rep", Type: FieldRepeater, SubFields: []Field{{Key: "inner", Type: FieldRepeater}}}}, true},
		{"section with condition on prior field", []Field{{Key: "a", Type: FieldText}, {Type: FieldSection, Label: "More", VisibleIf: &ConditionGroup{Rules: []Rule{{Field: "a", Op: OpNotEmpty}}}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateSchema(tc.fields); (err != nil) != tc.wantErr {
				t.Fatalf("ValidateSchema err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestVisibleFields_SectionGating(t *testing.T) {
	fields := []Field{
		{Key: "has_pet", Type: FieldConsent, Label: "I have a pet"},
		{Type: FieldSection, Label: "Pet details", VisibleIf: &ConditionGroup{Rules: []Rule{{Field: "has_pet", Op: OpEquals, Value: "true"}}}},
		{Key: "pet_name", Type: FieldText, Label: "Pet name", Required: true},
	}
	// Section hidden → pet_name hidden
	if hasField(VisibleFields(fields, map[string]any{}), "pet_name") {
		t.Fatal("pet_name should be hidden when its section is hidden")
	}
	// Section shown → pet_name visible
	if !hasField(VisibleFields(fields, map[string]any{"has_pet": true}), "pet_name") {
		t.Fatal("pet_name should be visible when its section is shown")
	}
}

func TestValidateAnswers_Matrix(t *testing.T) {
	fields := []Field{{
		Key: "grid", Type: FieldMatrix, Required: true,
		Rows:    []MatrixRow{{Key: "speed", Label: "Speed"}, {Key: "price", Label: "Price"}},
		Columns: []FieldOption{{Value: "good"}, {Value: "bad"}},
	}}
	vis := VisibleFields(fields, map[string]any{})
	// Missing a row → error
	if errs := validateAnswers(vis, map[string]any{"grid": map[string]any{"speed": "good"}}); !hasErr(errs, "grid") {
		t.Fatal("expected error for incomplete required matrix")
	}
	// Invalid column → error
	if errs := validateAnswers(vis, map[string]any{"grid": map[string]any{"speed": "good", "price": "nope"}}); !hasErr(errs, "grid") {
		t.Fatal("expected error for invalid matrix selection")
	}
	// Complete + valid → no error
	if errs := validateAnswers(vis, map[string]any{"grid": map[string]any{"speed": "good", "price": "bad"}}); len(errs) != 0 {
		t.Fatalf("expected no error, got %v", errs)
	}
}

func TestValidateAnswers_Repeater(t *testing.T) {
	one := 1
	fields := []Field{{
		Key: "guests", Type: FieldRepeater, MinItems: &one,
		SubFields: []Field{{Key: "name", Type: FieldText, Label: "Name", Required: true}, {Key: "email", Type: FieldEmail, Label: "Email"}},
	}}
	vis := VisibleFields(fields, map[string]any{})
	// Below minItems → error
	if errs := validateAnswers(vis, map[string]any{"guests": []any{}}); !hasErr(errs, "guests") {
		t.Fatal("expected error when below minItems")
	}
	// Item missing required subfield → error
	if errs := validateAnswers(vis, map[string]any{"guests": []any{map[string]any{"email": "a@b.com"}}}); !hasErr(errs, "guests") {
		t.Fatal("expected error for item missing required name")
	}
	// Valid item → no error
	if errs := validateAnswers(vis, map[string]any{"guests": []any{map[string]any{"name": "Ann", "email": "a@b.com"}}}); len(errs) != 0 {
		t.Fatalf("expected no error, got %v", errs)
	}
}

func f64(v float64) *float64 { return &v }

func hasField(fs []Field, key string) bool {
	for _, f := range fs {
		if f.Key == key {
			return true
		}
	}
	return false
}

func keysOf(fs []Field) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Key
	}
	return out
}

func hasErr(errs []FieldError, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

func TestValidateRedirectURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty is allowed", "", false},
		{"same-origin path", "/thank-you", false},
		{"nested path with query", "/pages/thanks?ref=form", false},
		{"absolute https", "https://shop.example.com/thanks", false},
		{"absolute http", "http://shop.example.com/thanks", false},
		{"protocol-relative host", "//evil.example.com", true},
		{"backslash protocol-relative", "/\\evil.example.com", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"data scheme", "data:text/html,<script>alert(1)</script>", true},
		{"bare host without scheme", "evil.example.com", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRedirectURL(tc.url)
			if tc.wantErr != (err != nil) {
				t.Fatalf("validateRedirectURL(%q) error = %v, wantErr %v", tc.url, err, tc.wantErr)
			}
		})
	}
}
