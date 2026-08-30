package cms

import (
	"encoding/json"
	"strings"
	"testing"
)

func rawJSON(t *testing.T, v any) json.RawMessage { // marshals a literal for a fixture, since DefaultValue and Validation are both JSONB
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return b
}

func attr(slug string, it InputType, translatable bool, def any) PageTypeAttribute {
	a := Attribute{Slug: slug, InputType: it, IsTranslatable: translatable}
	if def != nil {
		b, _ := json.Marshal(def)
		a.DefaultValue = b
	}
	return PageTypeAttribute{Attribute: a}
}

func TestDecodeJSONMapFailsSoftToAnEmptyMap(t *testing.T) { // callers index the result directly, so a nil map would panic on write
	for name, raw := range map[string]json.RawMessage{
		"nil":       nil,
		"empty":     json.RawMessage(""),
		"null":      json.RawMessage("null"),
		"malformed": json.RawMessage("{not json"),
		"an array":  json.RawMessage(`[1,2]`),
	} {
		got := decodeJSONMap(raw)
		if got == nil {
			t.Errorf("%s: decodeJSONMap = nil, want an empty map", name)
			continue
		}
		if len(got) != 0 {
			t.Errorf("%s: decodeJSONMap = %v, want it empty", name, got)
		}
		got["probe"] = 1 // must be writable
	}

	real := decodeJSONMap(json.RawMessage(`{"a":1,"b":"x"}`))
	if len(real) != 2 || real["b"] != "x" {
		t.Errorf("decodeJSONMap = %v, want both keys decoded", real)
	}
}

func TestIsEmpty(t *testing.T) { // the required-field gate reads this, so counting a whitespace-only string as filled would let a blank required field save
	for name, v := range map[string]any{
		"nil":     nil,
		"empty":   "",
		"spaces":  "   ",
		"tab":     "\t",
		"newline": "\n",
	} {
		if !isEmpty(v) {
			t.Errorf("%s: isEmpty = false, want true", name)
		}
	}
	for name, v := range map[string]any{
		"text":       "a",
		"padded":     " a ",
		"zero":       0,
		"false":      false,
		"empty map":  map[string]any{},
		"zero float": 0.0,
	} {
		if isEmpty(v) {
			t.Errorf("%s: isEmpty = true, want false - only nil and blank strings are empty", name)
		}
	}
}

func TestApplyAttributeDefaultsSeedsOnlyMissingKeys(t *testing.T) { // a default must never overwrite what the author supplied, including a deliberate empty string
	pt := PageType{Attributes: []PageTypeAttribute{
		attr("subtitle", InputText, false, "Default subtitle"),
		attr("cta", InputText, false, "Click me"),
	}}
	scalar := json.RawMessage(`{"cta":""}`) // deliberately blank, not absent

	gotScalar, _ := applyAttributeDefaults(pt, scalar, nil)
	m := decodeJSONMap(gotScalar)
	if m["subtitle"] != "Default subtitle" {
		t.Errorf("subtitle = %v, want the default seeded", m["subtitle"])
	}
	if m["cta"] != "" {
		t.Errorf("cta = %v, want the author's blank kept rather than defaulted", m["cta"])
	}
}

func TestApplyAttributeDefaultsRoutesTranslatablesToContent(t *testing.T) { // a translatable default in the scalar bag would be shared across locales instead of per-locale
	pt := PageType{Attributes: []PageTypeAttribute{
		attr("layout", InputText, false, "wide"),     // scalar
		attr("headline", InputText, true, "Welcome"), // translatable
	}}

	gotScalar, gotContent := applyAttributeDefaults(pt, nil, nil)
	scalarMap, contentMap := decodeJSONMap(gotScalar), decodeJSONMap(gotContent)

	if scalarMap["layout"] != "wide" {
		t.Errorf("scalar bag = %v, want the non-translatable default", scalarMap)
	}
	if _, leaked := scalarMap["headline"]; leaked {
		t.Error("a translatable default landed in the scalar bag, so every locale would share one value")
	}
	if contentMap["headline"] != "Welcome" {
		t.Errorf("content bag = %v, want the translatable default", contentMap)
	}
	if _, leaked := contentMap["layout"]; leaked {
		t.Error("a scalar default landed in the content bag")
	}
}

func TestApplyAttributeDefaultsReturnsInputsUnchangedWhenNothingSeeded(t *testing.T) { // the required-field gate distinguishes nil from {}, so seeding nothing must preserve nil-ness rather than materializing an empty object
	pt := PageType{Attributes: []PageTypeAttribute{
		attr("no_default", InputText, false, nil),
		attr("a_select", InputSelect, false, "x"),         // select values live elsewhere
		attr("a_relation", InputRelationship, false, "y"), // relationships too
	}}

	gotScalar, gotContent := applyAttributeDefaults(pt, nil, nil)
	if gotScalar != nil {
		t.Errorf("scalar = %q, want nil preserved when nothing was seeded", gotScalar)
	}
	if gotContent != nil {
		t.Errorf("content = %q, want nil preserved", gotContent)
	}
}

func TestApplyAttributeDefaultsSkipsSelectAndRelationshipTypes(t *testing.T) { // their values are rows, not inline scalars, so a JSON default would write a value nothing can resolve
	for _, it := range []InputType{InputSelect, InputMultiSelect, InputRelationship} {
		pt := PageType{Attributes: []PageTypeAttribute{attr("field", it, false, "should-not-seed")}}
		gotScalar, _ := applyAttributeDefaults(pt, nil, nil)
		if gotScalar != nil {
			t.Errorf("%s seeded %q, want it skipped", it, gotScalar)
		}
	}
}

func TestApplyAttributeDefaultsIgnoresAMalformedDefault(t *testing.T) { // a hand-edited default_value must not abort the create
	pt := PageType{Attributes: []PageTypeAttribute{
		{Attribute: Attribute{Slug: "broken", InputType: InputText, DefaultValue: json.RawMessage("{not json")}},
		attr("good", InputText, false, "fine"),
	}}
	gotScalar, _ := applyAttributeDefaults(pt, nil, nil)
	m := decodeJSONMap(gotScalar)
	if _, seeded := m["broken"]; seeded {
		t.Error("a malformed default was seeded")
	}
	if m["good"] != "fine" {
		t.Errorf("the valid default was dropped alongside it: %v", m)
	}
}

func TestApplyAttributeDefaultsSeedsNonStringTypes(t *testing.T) { // booleans and numbers default too, and a false or 0 default is a real value the gate must not treat as absent
	pt := PageType{Attributes: []PageTypeAttribute{
		attr("featured", InputBoolean, false, false),
		attr("order", InputNumber, false, 0),
		attr("tags", InputJSON, false, map[string]any{"k": "v"}),
	}}
	gotScalar, _ := applyAttributeDefaults(pt, nil, nil)
	m := decodeJSONMap(gotScalar)
	if m["featured"] != false {
		t.Errorf("featured = %v, want the false default seeded", m["featured"])
	}
	if m["order"] != float64(0) {
		t.Errorf("order = %v, want the 0 default seeded", m["order"])
	}
	if nested, ok := m["tags"].(map[string]any); !ok || nested["k"] != "v" {
		t.Errorf("tags = %v, want the JSON default seeded", m["tags"])
	}
}

func numberAttr(t *testing.T, min, max *float64) Attribute {
	t.Helper()
	rules := map[string]any{}
	if min != nil {
		rules["min"] = *min
	}
	if max != nil {
		rules["max"] = *max
	}
	return Attribute{Slug: "n", InputType: InputNumber, Validation: rawJSON(t, rules)}
}

func TestValidateAttributeRulesBoundsNumbers(t *testing.T) { // the rules are what stop an editor saving a value the storefront can't render
	lo, hi := 1.0, 10.0
	a := numberAttr(t, &lo, &hi)

	for _, ok := range []any{1.0, 5.0, 10.0, 7, int64(3)} {
		if err := validateAttributeRules(a, ok); err != nil {
			t.Errorf("validateAttributeRules(%v) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []any{0.0, 0.99, 10.01, 100.0} {
		if err := validateAttributeRules(a, bad); err == nil {
			t.Errorf("validateAttributeRules(%v) = nil, want it refused", bad)
		}
	}
	// a value of the wrong Go type is not the rules' business - coerceFieldValue already gated it
	if err := validateAttributeRules(a, "not a number"); err != nil {
		t.Errorf("a non-numeric value = %v, want nil rather than a bounds error", err)
	}
}

func TestValidateAttributeRulesBoundsStringLengthAndPattern(t *testing.T) {
	minLen, maxLen := 3, 8
	a := Attribute{Slug: "s", InputType: InputText, Validation: rawJSON(t, map[string]any{
		"minLength": minLen, "maxLength": maxLen, "regex": "^[a-z]+$",
	})}

	if err := validateAttributeRules(a, "abcd"); err != nil {
		t.Errorf("a conforming value = %v, want nil", err)
	}
	for name, bad := range map[string]string{
		"too short":     "ab",
		"too long":      "abcdefghi",
		"pattern":       "ABCD",
		"mixed pattern": "abc1",
	} {
		if err := validateAttributeRules(a, bad); err == nil {
			t.Errorf("%s (%q) was accepted, want it refused", name, bad)
		}
	}
}

func TestValidateAttributeRulesAppliesToEveryTextFamilyType(t *testing.T) { // a rule set on a textarea or rich text has to bind the same as on a plain text field
	for _, it := range []InputType{InputText, InputTextarea, InputURL, InputRichText} {
		a := Attribute{Slug: "s", InputType: it, Validation: rawJSON(t, map[string]any{"maxLength": 3})}
		if err := validateAttributeRules(a, "abcd"); err == nil {
			t.Errorf("%s did not enforce maxLength", it)
		}
		if err := validateAttributeRules(a, "abc"); err != nil {
			t.Errorf("%s refused a conforming value: %v", it, err)
		}
	}
}

func TestValidateAttributeRulesIgnoresMalformedOrAbsentRules(t *testing.T) { // malformed validation JSON must not block saves, which is the documented fail-open
	for name, a := range map[string]Attribute{
		"no rules":        {Slug: "s", InputType: InputText},
		"empty rules":     {Slug: "s", InputType: InputText, Validation: json.RawMessage("")},
		"malformed rules": {Slug: "s", InputType: InputText, Validation: json.RawMessage("{not json")},
	} {
		if err := validateAttributeRules(a, "anything at all"); err != nil {
			t.Errorf("%s: validateAttributeRules = %v, want nil", name, err)
		}
	}
	// a nil value is nothing to validate, whatever the rules say
	strict := Attribute{Slug: "s", InputType: InputText, Validation: rawJSON(t, map[string]any{"minLength": 5})}
	if err := validateAttributeRules(strict, nil); err != nil {
		t.Errorf("a nil value = %v, want nil - the required gate handles absence", err)
	}
}

func TestValidateAttributeRulesIgnoresARegexThatCannotCompile(t *testing.T) { // a bad pattern is an admin typo, and refusing every save would lock the field
	a := Attribute{Slug: "s", InputType: InputText, Validation: rawJSON(t, map[string]any{"regex": "([unclosed"})}
	if err := validateAttributeRules(a, "anything"); err != nil {
		t.Errorf("an uncompilable regex = %v, want it ignored", err)
	}
}

func TestValidateAttributeRulesSkipsTypesWithNoRuleFamily(t *testing.T) { // a length rule on a boolean is meaningless, and applying it would refuse every value
	for _, it := range []InputType{InputBoolean, InputJSON, InputSelect, InputRelationship} {
		a := Attribute{Slug: "x", InputType: it, Validation: rawJSON(t, map[string]any{"minLength": 100, "min": 999})}
		if err := validateAttributeRules(a, true); err != nil {
			t.Errorf("%s: validateAttributeRules = %v, want nil", it, err)
		}
	}
}

func TestSplitPrimaryContentPrefersTheDefaultLocale(t *testing.T) { // the primary row is what the admin form opens on, so picking the wrong one shows a translation as the source
	all := []PageContent{
		{Locale: "de", Title: "Willkommen"},
		{Locale: "en", Title: "Welcome"},
		{Locale: "fr", Title: "Bienvenue"},
	}

	primary, others := splitPrimaryContent(all, "en")
	if primary == nil || primary.Locale != "en" {
		t.Fatalf("primary = %+v, want the en row", primary)
	}
	if len(others) != 2 {
		t.Fatalf("%d others, want the remaining two", len(others))
	}
	for _, o := range others {
		if o.Locale == "en" {
			t.Error("the primary row is duplicated into others")
		}
	}
}

func TestSplitPrimaryContentFallsBackToTheFirstRow(t *testing.T) { // a page with no row for the default locale still has to render something rather than nothing
	all := []PageContent{{Locale: "de", Title: "Willkommen"}, {Locale: "fr", Title: "Bienvenue"}}

	for name, locale := range map[string]string{"absent locale": "en", "no locale asked": ""} {
		primary, others := splitPrimaryContent(all, locale)
		if primary == nil || primary.Locale != "de" {
			t.Errorf("%s: primary = %+v, want the first row", name, primary)
		}
		if len(others) != 1 || others[0].Locale != "fr" {
			t.Errorf("%s: others = %+v, want just the fr row", name, others)
		}
	}
}

func TestSplitPrimaryContentOnEmptyInput(t *testing.T) { // callers dereference primary, so a page with no content must report nil rather than a zero row
	primary, others := splitPrimaryContent(nil, "en")
	if primary != nil {
		t.Errorf("primary = %+v, want nil", primary)
	}
	if others != nil {
		t.Errorf("others = %+v, want nil", others)
	}
}

func TestSplitPrimaryContentWithASingleRow(t *testing.T) {
	primary, others := splitPrimaryContent([]PageContent{{Locale: "en", Title: "Only"}}, "en")
	if primary == nil || primary.Locale != "en" {
		t.Fatalf("primary = %+v, want the only row", primary)
	}
	if len(others) != 0 {
		t.Errorf("others = %+v, want none", others)
	}
}

func TestValidateSlugPathChecksEverySegment(t *testing.T) { // a root like "help/articles" is multi-segment, and validating only the whole string would let a bad segment through
	for _, ok := range []string{"help", "help/articles", "a/b/c", "getting-started/faq"} {
		if err := validateSlugPath(ok); err != nil {
			t.Errorf("validateSlugPath(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "help//articles", "help/Articles", "help/a b", "/help", "help/"} {
		if err := validateSlugPath(bad); err == nil {
			t.Errorf("validateSlugPath(%q) = nil, want it refused", bad)
		}
	}
}

func TestSlugifyTag(t *testing.T) { // tags are created from free-typed names, and the slug is what pages store - so it has to be stable and URL-safe
	for name, tc := range map[string]struct{ in, want string }{
		"plain":       {"news", "news"},
		"spaces":      {"Product News", "product-news"},
		"punctuation": {"News & Updates!", "news-updates"},
		"padded":      {"  news  ", "news"},
		"runs":        {"a   ---   b", "a-b"},
		"leading":     {"---news", "news"},
		"trailing":    {"news---", "news"},
		"digits":      {"2026 Roundup", "2026-roundup"},
		"unusable":    {"!!!", ""},
		"empty":       {"", ""},
	} {
		if got := slugifyTag(tc.in); got != tc.want {
			t.Errorf("%s: slugifyTag(%q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}

func TestSlugifyTagCapsLengthWithoutATrailingDash(t *testing.T) { // the column is bounded, and truncating mid-separator would leave a slug ending in a dash
	got := slugifyTag(strings.Repeat("ab ", 40))
	if len(got) > 64 {
		t.Errorf("slugifyTag produced %d chars, want it capped at 64", len(got))
	}
	if len(got) > 0 && (got[0] == '-' || got[len(got)-1] == '-') {
		t.Errorf("slugifyTag = %q, want no leading or trailing dash after the cap", got)
	}
}

func TestSlugifyTagIsIdempotent(t *testing.T) { // re-saving a tag must not drift its slug, or the pages referencing it orphan
	for _, in := range []string{"Product News", "news", "News & Updates!", "2026 Roundup"} {
		once := slugifyTag(in)
		if twice := slugifyTag(once); twice != once {
			t.Errorf("slugifyTag(slugifyTag(%q)) = %q, want %q", in, twice, once)
		}
	}
}

func TestCloneBodyReIDsTheBlocksItCopies(t *testing.T) { // duplicating a page has to break block identity, or edits to the copy would address the original's blocks
	orig := json.RawMessage(`{"version":1,"blocks":[{"id":"a","type":"hero"},{"id":"b","type":"section","blocks":[{"id":"c","type":"rich_text"}]}]}`)

	got := cloneBody(orig)
	for _, oldID := range []string{`"id":"a"`, `"id":"b"`, `"id":"c"`} {
		if strings.Contains(string(got), oldID) {
			t.Errorf("clone still carries %s, so the copy shares block identity with the original", oldID)
		}
	}
	if !strings.Contains(string(got), `"hero"`) || !strings.Contains(string(got), `"rich_text"`) {
		t.Errorf("clone = %s, want the block types preserved", got)
	}
	if string(got) == string(orig) {
		t.Error("cloneBody returned the body unchanged, so nothing was re-identified")
	}
}

func TestCloneBodyPassesLegacyBodiesThrough(t *testing.T) { // a freeform or Lexical body is not a block document, and rewriting it would corrupt authored content
	for name, raw := range map[string]json.RawMessage{
		"plain object": json.RawMessage(`{"root":{"children":[]}}`),
		"empty":        nil,
	} {
		got := cloneBody(raw)
		if string(got) != string(raw) {
			t.Errorf("%s: cloneBody = %s, want it returned unchanged", name, got)
		}
	}
}
