package review

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSlugNameOrderPreservesTaxonomyOrder(t *testing.T) { // the insights chart renders in this order, so reshuffling it makes an unchanged report look different on every refetch
	cats := []Category{
		{ID: uuid.New(), Slug: "fit", Name: "Fit"},
		{ID: uuid.New(), Slug: "quality", Name: "Quality"},
		{ID: uuid.New(), Slug: "delivery", Name: "Delivery"},
	}
	name, order := slugNameOrder(cats)

	if len(order) != 3 || order[0] != "fit" || order[1] != "quality" || order[2] != "delivery" {
		t.Errorf("order = %v, want the taxonomy order [fit quality delivery]", order)
	}
	for _, c := range cats {
		if name[c.Slug] != c.Name {
			t.Errorf("name[%q] = %q, want %q", c.Slug, name[c.Slug], c.Name)
		}
	}
}

func TestSlugNameOrderTagsMatchesTheCategoryShape(t *testing.T) { // the tag breakdown reuses the same seeding, so a divergence would order one panel differently to the other
	tags := []ModerationTag{
		{ID: uuid.New(), Slug: "profanity", Name: "Profanity"},
		{ID: uuid.New(), Slug: "spam", Name: "Spam"},
	}
	name, order := slugNameOrderTags(tags)
	if len(order) != 2 || order[0] != "profanity" || order[1] != "spam" {
		t.Errorf("order = %v, want [profanity spam]", order)
	}
	if name["spam"] != "Spam" {
		t.Errorf("name[spam] = %q, want Spam", name["spam"])
	}
}

func TestSlugNameOrderOnEmptyInput(t *testing.T) { // callers index and append to both, so neither may come back nil
	name, order := slugNameOrder(nil)
	if name == nil {
		t.Error("name map = nil, want an empty map callers can write to")
	}
	if order == nil {
		t.Error("order = nil, want an empty slice callers can append to")
	}
	name["probe"] = "x"
	_ = append(order, "probe")

	tagName, tagOrder := slugNameOrderTags(nil)
	if tagName == nil || tagOrder == nil {
		t.Error("slugNameOrderTags(nil) returned a nil map or slice")
	}
}

func TestEnsureSlugAddsAnUnknownSlugNamedAfterItself(t *testing.T) { // a category assigned to reviews and then DELETED still has counts, and dropping it would silently understate the report
	name, order := slugNameOrder([]Category{{Slug: "fit", Name: "Fit"}})

	name, order = ensureSlug(name, order, "deleted-category")
	if name["deleted-category"] != "deleted-category" {
		t.Errorf("name = %q, want the slug standing in for the missing name", name["deleted-category"])
	}
	if len(order) != 2 || order[1] != "deleted-category" {
		t.Errorf("order = %v, want the unknown slug appended after the known one", order)
	}
}

func TestEnsureSlugIsIdempotent(t *testing.T) { // Insights calls it once per slug in the current window and again for the prior one, so a slug in both must not appear twice in the chart
	name, order := slugNameOrder([]Category{{Slug: "fit", Name: "Fit"}})

	for range 3 {
		name, order = ensureSlug(name, order, "fit")
		name, order = ensureSlug(name, order, "unknown")
	}
	if len(order) != 2 {
		t.Errorf("order = %v, want each slug listed once", order)
	}
	if name["fit"] != "Fit" {
		t.Errorf("name[fit] = %q, want the real name preserved rather than overwritten by the slug", name["fit"])
	}
}

func TestEnsureSlugNeverOverwritesAKnownName(t *testing.T) { // the taxonomy name is what the operator reads - replacing it with the raw slug would make every panel look unlabelled
	name, order := slugNameOrder([]Category{{Slug: "fit", Name: "Fit"}, {Slug: "quality", Name: "Quality"}})
	name, order = ensureSlug(name, order, "fit")
	name, order = ensureSlug(name, order, "quality")

	if name["fit"] != "Fit" || name["quality"] != "Quality" {
		t.Errorf("names = %v, want the taxonomy names intact", name)
	}
	if len(order) != 2 {
		t.Errorf("order = %v, want no duplicates", order)
	}
}

func TestEnsureSlugHandlesTheEmptySlug(t *testing.T) { // "" is the uncategorized bucket, which Insights adds separately - ensureSlug must treat it as any other key rather than skipping it
	name, order := slugNameOrder(nil)
	name, order = ensureSlug(name, order, "")
	if _, ok := name[""]; !ok {
		t.Error("the empty slug was not recorded")
	}
	if len(order) != 1 {
		t.Errorf("order = %v, want the empty slug listed once", order)
	}
	_, order = ensureSlug(name, order, "")
	if len(order) != 1 {
		t.Errorf("order = %v, want it still listed once", order)
	}
}

func TestTagsJSONRoundTrips(t *testing.T) { // the column is jsonb and the moderation UI writes through it, so a shape change on either side loses a review's tags
	for name, in := range map[string][]string{
		"several": {"profanity", "spam"},
		"one":     {"spam"},
		"empty":   {},
	} {
		raw := tagsJSON(in)
		if !json.Valid(raw) {
			t.Errorf("%s: tagsJSON = %q, which is not valid JSON", name, raw)
			continue
		}
		got := parseTagsJSON(raw)
		if len(got) != len(in) {
			t.Errorf("%s: round-tripped to %v, want %v", name, got, in)
			continue
		}
		for i := range in {
			if got[i] != in[i] {
				t.Errorf("%s: round-tripped to %v, want %v", name, got, in)
				break
			}
		}
	}
}

func TestTagsJSONEncodesNilAsAnEmptyArray(t *testing.T) { // the column is NOT NULL jsonb, so nil has to serialize as [] rather than the null literal
	raw := tagsJSON(nil)
	if string(raw) != "[]" {
		t.Errorf("tagsJSON(nil) = %q, want []", raw)
	}
	if !json.Valid(raw) {
		t.Errorf("tagsJSON(nil) = %q, which is not valid JSON", raw)
	}
}

func TestParseTagsJSONFailsSoftToNoTags(t *testing.T) { // a hand-edited or malformed value must read as "no tags" rather than failing the review read
	for name, raw := range map[string]json.RawMessage{
		"nil":         nil,
		"empty":       json.RawMessage(""),
		"malformed":   json.RawMessage("[not json"),
		"null":        json.RawMessage("null"),
		"an object":   json.RawMessage(`{"a":1}`),
		"a string":    json.RawMessage(`"spam"`),
		"mixed types": json.RawMessage(`["spam",1]`),
	} {
		if got := parseTagsJSON(raw); got != nil {
			t.Errorf("%s: parseTagsJSON = %v, want nil", name, got)
		}
	}
	if got := parseTagsJSON(json.RawMessage(`[]`)); len(got) != 0 {
		t.Errorf("an empty array = %v, want no tags", got)
	}
}

func TestReviewTextGathersTitleBodyAndFreeTextAnswers(t *testing.T) { // the AI classifier and the moderation search both read this, so a dropped answer hides content from both
	rv := Review{
		Title: "Great tires",
		Body:  "Quiet on the motorway.",
		Answers: []Answer{
			{TextValue: "Fitted easily"},
			{TextValue: "   "}, // whitespace only, contributes nothing
			{TextValue: ""},    // a rating answer carries no text
			{TextValue: "Would rebuy"},
		},
	}

	got := reviewText(rv)
	for _, want := range []string{"Great tires", "Quiet on the motorway.", "Fitted easily", "Would rebuy"} {
		if !strings.Contains(got, want) {
			t.Errorf("reviewText = %q, want it to contain %q", got, want)
		}
	}
	// the blank answers must not leave a run of spaces the classifier has to cope with
	if strings.Contains(got, "  ") {
		t.Errorf("reviewText = %q, want no doubled spaces from the blank answers", got)
	}
	if strings.TrimSpace(got) != got {
		t.Errorf("reviewText = %q, want it trimmed", got)
	}
}

func TestReviewBodyTextExcludesTheTitle(t *testing.T) { // the body text feeds the summary separately from the title, so folding the title in would double it
	rv := Review{Title: "Great tires", Body: "Quiet.", Answers: []Answer{{TextValue: "Fitted easily"}}}
	got := reviewBodyText(rv)
	if strings.Contains(got, "Great tires") {
		t.Errorf("reviewBodyText = %q, want the title excluded", got)
	}
	if !strings.Contains(got, "Quiet.") || !strings.Contains(got, "Fitted easily") {
		t.Errorf("reviewBodyText = %q, want the body and the answer", got)
	}
}

func TestReviewTextOnAnEmptyReview(t *testing.T) { // a rating-only review has no text at all, and the callers branch on emptiness
	for name, rv := range map[string]Review{
		"nothing":        {},
		"blank fields":   {Title: "  ", Body: "  "},
		"rating answers": {Answers: []Answer{{TextValue: ""}, {TextValue: "   "}}},
	} {
		if got := reviewText(rv); got != "" {
			t.Errorf("%s: reviewText = %q, want empty", name, got)
		}
		if got := reviewBodyText(rv); got != "" {
			t.Errorf("%s: reviewBodyText = %q, want empty", name, got)
		}
	}
}

func TestStatusIsValid(t *testing.T) { // only 'approved' surfaces publicly, so an unrecognized status must not pass validation and land in the column
	for _, ok := range []Status{StatusPending, StatusApproved, StatusRejected} {
		if !ok.IsValid() {
			t.Errorf("%q.IsValid() = false, want true", ok)
		}
	}
	for _, bad := range []Status{"", "APPROVED", "published", "spam", "Pending"} {
		if bad.IsValid() {
			t.Errorf("%q.IsValid() = true, want false", bad)
		}
	}
}

func TestInputTypeIsValid(t *testing.T) {
	for _, ok := range []InputType{InputTypeRating1To5, InputTypeYesNo, InputTypeFreeText} {
		if !ok.IsValid() {
			t.Errorf("%q.IsValid() = false, want true", ok)
		}
	}
	for _, bad := range []InputType{"", "RATING_1_5", "slider", "rating"} {
		if bad.IsValid() {
			t.Errorf("%q.IsValid() = true, want false", bad)
		}
	}
}

func TestVoterIsExactlyOneIdentity(t *testing.T) { // a vote with both identities could be counted twice, and one with neither is unattributable - IsValid is the XOR that stops both
	id, tok := uuid.New(), uuid.New()

	customer := Voter{CustomerID: &id}
	if !customer.IsValid() || !customer.IsCustomer() || customer.IsAnon() {
		t.Errorf("a customer voter = %+v, want valid and not anonymous", customer)
	}
	anon := Voter{AnonToken: &tok}
	if !anon.IsValid() || !anon.IsAnon() || anon.IsCustomer() {
		t.Errorf("an anonymous voter = %+v, want valid and anonymous", anon)
	}
	if !anon.IsAnonymous() {
		t.Error("IsAnonymous disagrees with IsAnon")
	}

	both := Voter{CustomerID: &id, AnonToken: &tok}
	if both.IsValid() {
		t.Error("a voter carrying both identities is valid, so one vote could be counted twice")
	}
	if (Voter{}).IsValid() {
		t.Error("an empty voter is valid, so a vote could be unattributable")
	}
}
