package embedding

import (
	"encoding/json"
	"html"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/skeletg/skelet/apps/backend/core/internal/blockkit"
)

// A page passage aims at this many characters - a few paragraphs, about one topic - and only splits at a heading or a paragraph
const passageChars = 1200

// The props that hold words a shopper reads, as opposed to links, ids and settings
var proseKeys = map[string]bool{
	"heading": true, "title": true, "subtitle": true, "eyebrow": true, "text": true, "body": true, "content": true,
	"description": true, "caption": true, "quote": true, "question": true, "answer": true, "plain": true,
}

// A block's own heading starts a new passage, so a question lands on the section that answers it
var headingKeys = []string{"heading", "title", "question"}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// richText is the readable text of a rich-text value: its plain copy when it carries one, else its prose leaves in order
func richText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	if m, ok := v.(map[string]any); ok {
		if p, ok := m["plain"].(string); ok && strings.TrimSpace(p) != "" {
			return tidy(p)
		}
	}
	var parts []string
	collect(v, &parts)
	return tidy(strings.Join(parts, " "))
}

// collect walks a value in a stable order - arrays as they are, object keys sorted - so the same content always reads the same
func collect(v any, parts *[]string) {
	switch t := v.(type) {
	case string:
		*parts = append(*parts, t)
	case []any:
		for _, e := range t {
			collect(e, parts)
		}
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(t)) {
			switch val := t[k].(type) {
			case string:
				if k == "html" {
					*parts = append(*parts, html.UnescapeString(tagRe.ReplaceAllString(blockkit.SanitizeHTML(val), " ")))
				} else if proseKeys[k] {
					*parts = append(*parts, val)
				}
			case []any, map[string]any:
				collect(val, parts)
			}
		}
	}
}

// productText is one passage: what the product is, how the store files it, its public facts, then the description
type productText struct {
	name, kind, short, long string
	categories, facts       []string
}

func (p productText) String() string {
	var b strings.Builder
	line := func(s string) {
		if s = tidy(s); s != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(s)
		}
	}
	line(p.name)
	if p.kind != "" {
		line("Type: " + p.kind)
	}
	if len(p.categories) > 0 {
		line("Categories: " + strings.Join(p.categories, ", "))
	}
	for _, f := range p.facts {
		line(f)
	}
	line(p.short)
	if p.long != p.short {
		line(p.long)
	}
	return b.String()
}

// pagePassages splits a page into passages, each led by the page title and the heading it sits under. A page built from
// blocks splits at its headings, freeform text at its paragraphs
func pagePassages(title, meta string, body json.RawMessage) []string {
	title = tidy(title)
	var sections []section
	var doc blockkit.Document
	if json.Unmarshal(body, &doc) == nil && len(doc.Blocks) > 0 {
		blockSections(doc.Blocks, &sections)
	} else {
		for para := range strings.SplitSeq(richParagraphs(body), "\n\n") {
			sections = append(sections, section{text: tidy(para)})
		}
	}
	if meta = tidy(meta); meta != "" {
		sections = append([]section{{text: meta}}, sections...)
	}
	return pack(title, sections)
}

type section struct{ heading, text string }

func blockSections(blocks []blockkit.Block, out *[]section) {
	for _, b := range blocks {
		props := maps.Clone(b.Props)
		heading := ""
		for _, k := range headingKeys {
			if h, ok := props[k].(string); ok && tidy(h) != "" {
				heading = tidy(h)
				delete(props, k)
				break
			}
		}
		// Accordions, tabs and the like keep their items in a list - each item is a section of its own, an FAQ answer most often
		var items []any
		if list, ok := props["items"].([]any); ok {
			items = list
			delete(props, "items")
		}
		var parts []string
		collect(map[string]any(props), &parts)
		if text := tidy(strings.Join(parts, " ")); text != "" || heading != "" {
			*out = append(*out, section{heading: heading, text: text})
		}
		for _, it := range items {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			sub := blockkit.Block{Props: m}
			blockSections([]blockkit.Block{sub}, out)
		}
		blockSections(b.Blocks, out)
	}
}

// Freeform content keeps its paragraph breaks, so it can split there
func richParagraphs(raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		if p, ok := m["plain"].(string); ok {
			return p
		}
	}
	return richText(raw)
}

// pack joins sections into passages near passageChars, starting a fresh one at a heading once the current one has words
func pack(title string, sections []section) []string {
	var out []string
	var cur strings.Builder
	heading := ""
	flush := func() {
		if strings.TrimSpace(cur.String()) != "" {
			out = append(out, lead(title, heading)+cur.String())
		}
		cur.Reset()
	}
	for _, s := range sections {
		if s.heading != "" && cur.Len() > 0 {
			flush()
		}
		if s.heading != "" {
			heading = s.heading
		}
		for _, piece := range splitLong(s.text) {
			if cur.Len() > 0 && cur.Len()+len(piece) > passageChars {
				flush()
			}
			if cur.Len() > 0 {
				cur.WriteString("\n")
			}
			cur.WriteString(piece)
		}
	}
	flush()
	if len(out) == 0 && title != "" {
		out = append(out, title)
	}
	return out
}

func lead(title, heading string) string {
	switch {
	case title != "" && heading != "" && heading != title:
		return title + " - " + heading + "\n"
	case title != "":
		return title + "\n"
	case heading != "":
		return heading + "\n"
	}
	return ""
}

// splitLong cuts a section longer than a passage at sentence ends, so no passage grows without bound
func splitLong(text string) []string {
	if len(text) <= passageChars {
		return []string{text}
	}
	var out []string
	var cur strings.Builder
	for _, sentence := range strings.SplitAfter(text, ". ") {
		if cur.Len() > 0 && cur.Len()+len(sentence) > passageChars {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
		cur.WriteString(sentence)
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}

// tidy folds whitespace runs to one space and drops broken UTF-8, so the same words always hash the same
func tidy(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	return strings.Join(strings.Fields(s), " ")
}
