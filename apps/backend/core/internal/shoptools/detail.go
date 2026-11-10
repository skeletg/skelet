package shoptools

import (
	"strings"

	"github.com/google/uuid"
)

// The panel's take on a product is kept to what reads at a glance
const (
	maxSummaryChars   = 240
	maxHighlights     = 4
	maxPointTitle     = 40
	maxPointChars     = 140
	maxGoodFor        = 4
	maxTagChars       = 32
	maxKeepInMind     = 3
	maxKeepInMindText = 140
)

type pointArg struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Point is one highlight in a product panel: a few words of title and a line of detail
type Point struct {
	Title  string
	Detail string
}

// Detail is the product a reply explains, and the explanation the model laid out for its panel - empty when the panel came from the reply looking at one product
type Detail struct {
	ProductID  uuid.UUID
	Summary    string
	Highlights []Point
	GoodFor    []string
	KeepInMind []string
}

// detailFrom keeps what the model wrote for the panel within the panel's room, dropping anything left empty
func detailFrom(id uuid.UUID, a args) Detail {
	d := Detail{ProductID: id, Summary: clip(a.Summary, maxSummaryChars)}
	for _, p := range a.Highlights {
		if title := clip(p.Title, maxPointTitle); title != "" && len(d.Highlights) < maxHighlights {
			d.Highlights = append(d.Highlights, Point{Title: title, Detail: clip(p.Detail, maxPointChars)})
		}
	}
	d.GoodFor = clipAll(a.GoodFor, maxGoodFor, maxTagChars)
	d.KeepInMind = clipAll(a.KeepInMind, maxKeepInMind, maxKeepInMindText)
	return d
}

func clipAll(items []string, most, chars int) []string {
	var out []string
	for _, s := range items {
		if s = clip(strings.TrimSpace(s), chars); s != "" && len(out) < most {
			out = append(out, s)
		}
	}
	return out
}

func detailSchema() map[string]any {
	list := func(desc string, most int) map[string]any {
		return map[string]any{"type": "array", "maxItems": most, "items": map[string]any{"type": "string"}, "description": desc}
	}
	return schema(map[string]any{
		"id":      str("The product's id."),
		"summary": str("One sentence: what it is and who it suits."),
		"highlights": map[string]any{
			"type": "array", "maxItems": maxHighlights, "description": "What stands out, best first - each a few words of title and a short line.",
			"items": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"title"},
				"properties": map[string]any{"title": str("A few words, like \"Grips in the wet\"."), "detail": str("One short line backing it up.")},
			},
		},
		"good_for":     list("A few words each - who or what it suits, like \"Daily commutes\".", maxGoodFor),
		"keep_in_mind": list("Anything to weigh before buying, one short line each - out of stock, choices to make on its page, a size note.", maxKeepInMind),
	}, "id")
}
