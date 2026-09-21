package blockkit

import (
	"regexp"
	"sort"
	"strings"
)

var humanTextProps = map[string]bool{ // prop keys whose string values are worth indexing for search - machine props like href, src, assetId, slug and entityId are deliberately excluded, so the index holds words a person would search for rather than config values
	"title": true, "subtitle": true, "heading": true, "eyebrow": true,
	"text": true, "body": true, "caption": true, "label": true,
	"quote": true, "cite": true, "alt": true, "description": true,
	"value": true, "name": true, "author": true,
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

func PlainText(doc Document) string { // human-readable text joined by spaces, for full-text search indexing - it recurses child blocks and repeater item arrays, collects string values under humanTextProps and strips tags from `html` props, bounded by maxDepth, mirroring prepareBlocks' traversal but collecting strings instead of rendering
	var sb strings.Builder
	collectBlocksText(doc.Blocks, &sb, 1)
	return strings.TrimSpace(sb.String())
}

func collectBlocksText(blocks []Block, sb *strings.Builder, depth int) {
	if depth > maxDepth {
		return
	}
	for _, b := range blocks {
		collectPropsText(b.Props, sb)
		collectBlocksText(b.Blocks, sb, depth+1)
	}
}

func collectPropsText(props map[string]any, sb *strings.Builder) {
	for k, v := range props {
		switch val := v.(type) {
		case string:
			switch {
			case k == "html":
				writeText(sb, htmlTagRe.ReplaceAllString(val, " "))
			case humanTextProps[k]:
				writeText(sb, val)
			}
		case []any:
			for _, it := range val {
				if m, ok := it.(map[string]any); ok {
					collectPropsText(m, sb)
				}
			}
		case map[string]any:
			collectPropsText(val, sb)
		}
	}
}

func writeText(sb *strings.Builder, s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	if sb.Len() > 0 {
		sb.WriteByte(' ')
	}
	sb.WriteString(s)
}

var translatableKeys = func() map[string]bool { // prop keys whose RAW string values are translated - humanTextProps plus html, with tags preserved unlike PlainText which strips them
	m := map[string]bool{"html": true}
	for k := range humanTextProps {
		m[k] = true
	}
	return m
}()

func TranslatableStrings(doc Document) []string { // STABLE order - sorted prop keys, then child blocks, then repeater-item arrays; ApplyTranslations walks the identical order, so a caller can translate this slice and map it straight back
	var out []string
	walkTranslatable(doc.Blocks, 1, func(s string) string {
		out = append(out, s)
		return s // collect-only: leave the value unchanged
	})
	return out
}

func ApplyTranslations(doc Document, translations []string) Document { // fills the translatable slots in the same stable order TranslatableStrings produced them, preserving all structure - it mutates the given Document's prop maps in place, so callers pass a freshly-parsed doc
	i := 0
	walkTranslatable(doc.Blocks, 1, func(orig string) string {
		if i < len(translations) {
			t := translations[i]
			i++
			return t
		}
		i++
		return orig
	})
	return doc
}

func walkTranslatable(blocks []Block, depth int, next func(string) string) { // visits each non-empty translatable slot in stable order and replaces it with next(current)
	if depth > maxDepth {
		return
	}
	for bi := range blocks {
		translateProps(blocks[bi].Props, next)
		walkTranslatable(blocks[bi].Blocks, depth+1, next)
	}
}

func translateProps(props map[string]any, next func(string) string) {
	if props == nil {
		return
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch v := props[k].(type) {
		case string:
			if translatableKeys[k] && strings.TrimSpace(v) != "" {
				props[k] = next(v)
			}
		case []any:
			for _, it := range v {
				if m, ok := it.(map[string]any); ok {
					translateProps(m, next)
				}
			}
		case map[string]any:
			translateProps(v, next)
		}
	}
}
