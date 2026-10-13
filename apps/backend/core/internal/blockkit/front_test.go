package blockkit

import (
	"strings"
	"testing"
	"time"
)

var (
	frontCustom   = func(string, map[string]any) (string, bool) { return "<p>custom</p>", true }
	frontForms    = func(string) (map[string]any, bool) { return map[string]any{"fields": []any{}}, true }
	frontSnippets = func(string) ([]Block, bool) {
		return []Block{{ID: "s1", Type: TypeRichText, Props: map[string]any{"body": "snippet"}}}, true
	}
)

func rebuiltBlocks(vis map[string]any) Document {
	return Document{Version: 1, Blocks: []Block{
		{ID: "c", Type: "promo_custom", Props: map[string]any{"visibility": vis}},
		{ID: "f", Type: TypeForm, Props: map[string]any{"formId": "f1", "visibility": vis}},
		{ID: "r", Type: TypeRef, Props: map[string]any{"slug": "s", "visibility": vis}},
	}}
}

// Custom, form and ref blocks are rebuilt while preparing, and used to lose their rule before the filter could read it
func TestForFrontEnforcesVisibilityOnRebuiltBlocks(t *testing.T) {
	now := time.Now()
	for name, c := range map[string]struct {
		vis  map[string]any
		ec   EvalContext
		want int
	}{
		"expired schedule":         {map[string]any{"endAt": "2020-01-01T00:00:00Z"}, EvalContext{Now: now}, 0},
		"not started yet":          {map[string]any{"startAt": "2999-01-01T00:00:00Z"}, EvalContext{Now: now}, 0},
		"segment the viewer lacks": {map[string]any{"segments": []any{"vip"}}, EvalContext{Now: now}, 0},
		"segment the viewer has":   {map[string]any{"segments": []any{"vip"}}, EvalContext{Now: now, Segments: map[string]bool{"vip": true}}, 3},
	} {
		out := ForFront(rebuiltBlocks(c.vis), c.ec, frontSnippets, frontCustom, nil, nil, frontForms)
		if len(out.Blocks) != c.want {
			t.Errorf("%s: shipped %d blocks, want %d", name, len(out.Blocks), c.want)
		}
		for _, b := range out.Blocks {
			if v, ok := b.Props["visibility"].(map[string]any); ok && v["segments"] != nil {
				t.Errorf("%s: block %s shipped its audience rule", name, b.ID)
			}
		}
	}
}

func TestForFrontKeepsTheDeviceHintOnRebuiltBlocks(t *testing.T) {
	out := ForFront(rebuiltBlocks(map[string]any{"devices": []any{"mobile"}}), EvalContext{Now: time.Now()}, frontSnippets, frontCustom, nil, nil, frontForms)
	if len(out.Blocks) != 3 {
		t.Fatalf("shipped %d blocks, want 3", len(out.Blocks))
	}
	for _, b := range out.Blocks {
		v, _ := b.Props["visibility"].(map[string]any)
		if d, _ := v["devices"].([]string); len(d) != 1 || d[0] != "mobile" {
			t.Errorf("block %s (%s) lost its device hint: %v", b.ID, b.Type, b.Props["visibility"])
		}
	}
}

// A snippet block with its own rule keeps it, and the second filter enforces it
func TestForFrontEnforcesRulesInsideASnippet(t *testing.T) {
	snippets := func(string) ([]Block, bool) {
		return []Block{
			{ID: "live", Type: TypeRichText, Props: map[string]any{"body": "on"}},
			{ID: "gone", Type: TypeRichText, Props: map[string]any{"body": "off", "visibility": map[string]any{"endAt": "2020-01-01T00:00:00Z"}}},
		}, true
	}
	doc := Document{Version: 1, Blocks: []Block{{ID: "r", Type: TypeRef, Props: map[string]any{"slug": "s"}}}}
	out := ForFront(doc, EvalContext{Now: time.Now()}, snippets, nil, nil, nil, nil)
	if len(out.Blocks) != 1 || out.Blocks[0].ID != "live" {
		t.Errorf("shipped %+v, want only the snippet block still in its window", out.Blocks)
	}
}

func TestSanitizeHTMLLeavesTheMerchantsLinksFollowable(t *testing.T) {
	for in, want := range map[string]string{
		`<a href="/sale">Sale</a>`:                                `<a href="/sale">Sale</a>`,
		`<a href="https://partner.example" target="_blank">P</a>`: `target="_blank"`,
	} {
		got := SanitizeHTML(in)
		if strings.Contains(got, "nofollow") || !strings.Contains(got, want) {
			t.Errorf("SanitizeHTML(%s) = %s, want %s and no nofollow", in, got, want)
		}
	}
	if got := SanitizeHTML(`<a href="/x" target="_top">x</a>`); strings.Contains(got, "target") {
		t.Errorf("a target other than _blank survived: %s", got)
	}
	if got := SanitizeHTML(`<a href="javascript:alert(1)" target="_blank">x</a>`); strings.Contains(got, "javascript") {
		t.Errorf("relaxing links let a javascript: URL through: %s", got)
	}
}

func TestValidateRequiresAUniqueIDOnEveryBlock(t *testing.T) {
	for name, doc := range map[string]Document{
		"missing id":               {Blocks: []Block{{Type: TypeRichText}}},
		"duplicate at one level":   {Blocks: []Block{{ID: "a", Type: TypeRichText}, {ID: "a", Type: TypeDivider}}},
		"duplicate across nesting": {Blocks: []Block{{ID: "a", Type: TypeSection, Blocks: []Block{{ID: "a", Type: TypeRichText}}}}},
	} {
		if err := Validate(doc, nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	ok := Document{Blocks: []Block{{ID: "a", Type: TypeSection, Blocks: []Block{{ID: "b", Type: TypeRichText}}}}}
	if err := Validate(ok, nil); err != nil {
		t.Errorf("unique ids refused: %v", err)
	}
}

func TestPlainTextIndexesOnlyVisibleText(t *testing.T) {
	doc := Document{Blocks: []Block{{ID: "h", Type: TypeHTML, Props: map[string]any{
		"html": `<style>.x{color:red}</style><p>Fish &amp; chips</p><script>trackVisitor()</script>`,
	}}}}
	got := PlainText(doc)
	if got != "Fish & chips" {
		t.Errorf("PlainText = %q, want only the visible, decoded text", got)
	}
}

func TestPrepareForFrontDropsChildrenOfAnHTMLBlock(t *testing.T) {
	doc := Document{Blocks: []Block{{ID: "h", Type: TypeHTML, Props: map[string]any{"html": "<p>x</p>"},
		Blocks: []Block{{ID: "k", Type: TypeHTML, Props: map[string]any{"html": `<img src=x onerror=alert(1)>`}}}}}}
	if out := PrepareForFront(doc, nil, nil, nil, nil, nil); len(out.Blocks[0].Blocks) != 0 {
		t.Errorf("an html block shipped %d children", len(out.Blocks[0].Blocks))
	}
}

// A snippet block with its own rule doesn't inherit the ref's, so the ref's rule has to be enforced before it expands
func TestForFrontHidesASnippetWhoseRefIsOutOfItsWindow(t *testing.T) {
	snippets := func(string) ([]Block, bool) {
		return []Block{{ID: "s1", Type: TypeRichText, Props: map[string]any{"body": "x", "visibility": map[string]any{"devices": []any{"mobile"}}}}}, true
	}
	doc := Document{Version: 1, Blocks: []Block{{ID: "r", Type: TypeRef, Props: map[string]any{"slug": "s", "visibility": map[string]any{"endAt": "2020-01-01T00:00:00Z"}}}}}
	if out := ForFront(doc, EvalContext{Now: time.Now()}, snippets, nil, nil, nil, nil); len(out.Blocks) != 0 {
		t.Errorf("shipped %+v from a ref whose schedule ended", out.Blocks)
	}
}
