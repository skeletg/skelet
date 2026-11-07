package embedding

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRichTextReadsThePlainCopyOrTheProseInOrder(t *testing.T) {
	for raw, want := range map[string]string{
		`{"text": "<p>A tee</p>", "plain": "A  timeless\n tee."}`:                                                            "A timeless tee.",
		`{"root": {"children": [{"children": [{"text": "First"}, {"text": "second"}]}, {"children": [{"text": "third"}]}]}}`: "First second third",
		`"just words"`: "just words",
		`null`:         "",
		`{broken`:      "",
	} {
		if got := richText(json.RawMessage(raw)); got != want {
			t.Errorf("%s -> %q, want %q", raw, got, want)
		}
	}
}

func TestAProductReadsAsOnePassageOfItsFacts(t *testing.T) {
	got := productText{name: "Canvas Backpack", kind: "Bags", short: "A carry-on pack.", long: "A carry-on pack.",
		categories: []string{"Bags", "Travel"}, facts: []string{"Colour: Olive", "Volume: 28 l"}}.String()
	want := "Canvas Backpack\nType: Bags\nCategories: Bags, Travel\nColour: Olive\nVolume: 28 l\nA carry-on pack."
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestABlockPageSplitsAtItsHeadingsAndFAQItems(t *testing.T) {
	body := `{"version": 1, "blocks": [
		{"type": "hero", "props": {"heading": "Shipping", "subtitle": "Fast and tracked", "image": {"src": "/x.jpg", "alt": "a van"}}},
		{"type": "rich_text", "props": {"body": {"plain": "We ship to every EU country in 2-4 days."}}},
		{"type": "rich_text", "props": {"heading": "Outside the EU", "html": "<p>The UK and Switzerland take <b>5-7 days</b>.</p><script>x()</script>"}},
		{"type": "accordion", "props": {"items": [
			{"title": "Do you ship to Norway?", "body": "Yes, in a week."},
			{"title": "Is there customs?", "body": "Outside the EU, yes."}]}},
		{"type": "section", "blocks": [{"type": "rich_text", "props": {"body": {"plain": "Returns are free."}}}]}
	]}`
	got := pagePassages("Delivery", "How and where we ship.", json.RawMessage(body))
	want := []string{
		"Delivery\nHow and where we ship.",
		"Delivery - Shipping\nFast and tracked\nWe ship to every EU country in 2-4 days.",
		"Delivery - Outside the EU\nThe UK and Switzerland take 5-7 days .",
		"Delivery - Do you ship to Norway?\nYes, in a week.",
		"Delivery - Is there customs?\nOutside the EU, yes.\nReturns are free.",
	}
	if strings.Join(got, "\n---\n") != strings.Join(want, "\n---\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n---\n"), strings.Join(want, "\n---\n"))
	}
	if again := pagePassages("Delivery", "How and where we ship.", json.RawMessage(body)); strings.Join(again, "|") != strings.Join(got, "|") {
		t.Error("the same page split differently the second time, so its hashes would never match")
	}
}

func TestFreeformTextSplitsAtParagraphsAndLongOnesAtSentences(t *testing.T) {
	long := strings.Repeat("A sentence about returns. ", 100)
	got := pagePassages("Returns", "", json.RawMessage(`{"plain": "Short intro.\n\n`+long+`"}`))
	if len(got) < 3 {
		t.Fatalf("%d passages from a page of %d characters", len(got), len(long))
	}
	for _, p := range got {
		if !strings.HasPrefix(p, "Returns\n") || len(p) > passageChars+len("Returns\n")+40 {
			t.Errorf("passage %q has no title or runs long (%d)", p[:min(40, len(p))], len(p))
		}
	}
	if got := pagePassages("Empty", "", json.RawMessage(`{}`)); len(got) != 1 || got[0] != "Empty" {
		t.Errorf("a page with only a title = %q", got)
	}
}
