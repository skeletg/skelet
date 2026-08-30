package media

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// SVG is the one image format that is a program. It can carry <script>, on* handlers, external
// references and entity expansions, so an uploaded one is stored XSS unless it is cut down first
//
// The approach is allowlist-and-REBUILD, not search-and-remove: the document is parsed as XML, every
// element and attribute not on the lists below is dropped, and the output is written fresh from what
// survived. That matters because the usual way a sanitizer is defeated is a parser differential - the
// filter reads the bytes one way, the browser another. Nothing here is passed through, so the browser
// only ever sees markup this file produced
//
// Deliberately narrow: drawing elements only. No <script>, <style>, <foreignObject>, <a>, no
// animation, no filters, no external references of any kind. It is enough for a logo or an icon,
// which is what it exists for - not for arbitrary artwork
var (
	ErrNotSVG         = errors.New("media: not an SVG document")
	ErrSVGUnsanitized = errors.New("media: SVG could not be made safe")
)

const (
	maxSVGDepth    = 64
	maxSVGElements = 20000
)

// Shape, structure and paint only. <use> and gradient href are allowed but their references are
// forced to same-document fragments below
var allowedSVGElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "title": true, "desc": true,
	"symbol": true, "use": true,
	"path": true, "rect": true, "circle": true, "ellipse": true,
	"line": true, "polyline": true, "polygon": true,
	"text": true, "tspan": true,
	"linearGradient": true, "radialGradient": true, "stop": true,
	"clipPath": true, "mask": true, "pattern": true, "marker": true,
}

// Presentation attributes valid on most elements
var allowedSVGCommonAttrs = map[string]bool{
	"id": true, "class": true, "transform": true, "style": true,
	"fill": true, "fill-opacity": true, "fill-rule": true,
	"stroke": true, "stroke-width": true, "stroke-opacity": true, "stroke-linecap": true,
	"stroke-linejoin": true, "stroke-miterlimit": true, "stroke-dasharray": true, "stroke-dashoffset": true,
	"opacity": true, "color": true, "display": true, "visibility": true,
	"clip-path": true, "clip-rule": true, "mask": true,
	"font-family": true, "font-size": true, "font-style": true, "font-weight": true,
	"text-anchor": true, "letter-spacing": true, "word-spacing": true,
	"dominant-baseline": true, "alignment-baseline": true,
	"paint-order": true, "vector-effect": true, "shape-rendering": true, "fill-opacity-x": false,
}

// Geometry, per element - keeps a `cx` off a <rect> where it means nothing
var allowedSVGElementAttrs = map[string]map[string]bool{
	"svg":            {"viewBox": true, "width": true, "height": true, "preserveAspectRatio": true, "version": true, "xmlns": true},
	"symbol":         {"viewBox": true, "preserveAspectRatio": true},
	"use":            {"href": true, "x": true, "y": true, "width": true, "height": true},
	"path":           {"d": true, "pathLength": true},
	"rect":           {"x": true, "y": true, "width": true, "height": true, "rx": true, "ry": true},
	"circle":         {"cx": true, "cy": true, "r": true},
	"ellipse":        {"cx": true, "cy": true, "rx": true, "ry": true},
	"line":           {"x1": true, "y1": true, "x2": true, "y2": true},
	"polyline":       {"points": true},
	"polygon":        {"points": true},
	"text":           {"x": true, "y": true, "dx": true, "dy": true, "rotate": true, "textLength": true, "lengthAdjust": true},
	"tspan":          {"x": true, "y": true, "dx": true, "dy": true, "rotate": true, "textLength": true, "lengthAdjust": true},
	"linearGradient": {"x1": true, "y1": true, "x2": true, "y2": true, "gradientUnits": true, "gradientTransform": true, "spreadMethod": true, "href": true},
	"radialGradient": {"cx": true, "cy": true, "r": true, "fx": true, "fy": true, "gradientUnits": true, "gradientTransform": true, "spreadMethod": true, "href": true},
	"stop":           {"offset": true, "stop-color": true, "stop-opacity": true},
	"clipPath":       {"clipPathUnits": true},
	"mask":           {"maskUnits": true, "maskContentUnits": true, "x": true, "y": true, "width": true, "height": true},
	"pattern":        {"patternUnits": true, "patternContentUnits": true, "patternTransform": true, "x": true, "y": true, "width": true, "height": true, "viewBox": true},
	"marker":         {"markerUnits": true, "markerWidth": true, "markerHeight": true, "refX": true, "refY": true, "orient": true, "viewBox": true},
}

// SanitizeSVG parses src as XML and rebuilds it from the allowlists above, returning markup safe to
// serve as image/svg+xml. It fails rather than guesses: malformed XML, a non-<svg> root, or a
// document deeper or larger than the caps is rejected outright
func SanitizeSVG(src []byte) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(src))
	dec.Strict = true
	// An undefined entity is an error rather than a lookup, which is what closes XXE and the
	// billion-laughs expansion - neither can reach a resolver from here
	dec.Entity = xml.HTMLEntity

	var out bytes.Buffer
	var stack []string
	elements := 0
	seenRoot := false

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSVGUnsanitized, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if !seenRoot {
				if name != "svg" {
					return nil, fmt.Errorf("%w: root element is <%s>", ErrNotSVG, name)
				}
				seenRoot = true
			}
			elements++
			if elements > maxSVGElements {
				return nil, fmt.Errorf("%w: more than %d elements", ErrSVGUnsanitized, maxSVGElements)
			}
			// A dropped element takes its subtree with it: keeping the children of a <script> or
			// <foreignObject> would be how the content got back in
			if !allowedSVGElements[name] {
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("%w: %w", ErrSVGUnsanitized, err)
				}
				continue
			}
			if len(stack) >= maxSVGDepth {
				return nil, fmt.Errorf("%w: nested deeper than %d", ErrSVGUnsanitized, maxSVGDepth)
			}
			out.WriteByte('<')
			out.WriteString(name)
			if name == "svg" {
				// Emitted by us, not copied - every prefix is stripped, so the document is entirely
				// in the SVG namespace and there is no xlink: left to smuggle anything through
				out.WriteString(` xmlns="http://www.w3.org/2000/svg"`)
			}
			for _, a := range t.Attr {
				k, v, ok := sanitizeSVGAttr(name, a)
				if !ok {
					continue
				}
				out.WriteByte(' ')
				out.WriteString(k)
				out.WriteString(`="`)
				_ = xml.EscapeText(&out, []byte(v)) // writing to a bytes.Buffer cannot fail
				out.WriteByte('"')
			}
			out.WriteByte('>')
			stack = append(stack, name)

		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			out.WriteString("</")
			out.WriteString(stack[len(stack)-1])
			out.WriteByte('>')
			stack = stack[:len(stack)-1]

		case xml.CharData:
			// Text only survives inside elements that render it - stray character data elsewhere is
			// noise at best, and escaping it everywhere would bloat the output
			if len(stack) > 0 {
				switch stack[len(stack)-1] {
				case "text", "tspan", "title", "desc":
					_ = xml.EscapeText(&out, t)
				}
			}
		}
		// Comments, processing instructions and directives (including <!DOCTYPE ...>) are dropped by
		// falling through: none of them can carry anything we want to keep
	}

	if !seenRoot {
		return nil, ErrNotSVG
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("%w: %d unclosed element(s)", ErrSVGUnsanitized, len(stack))
	}
	return out.Bytes(), nil
}

// sanitizeSVGAttr decides one attribute: the returned name is always an unprefixed local name
func sanitizeSVGAttr(element string, a xml.Attr) (string, string, bool) {
	name := a.Name.Local
	lower := strings.ToLower(name)

	// Namespace machinery: only the SVG xmlns survives, and we write that ourselves
	if lower == "xmlns" || strings.EqualFold(a.Name.Space, "xmlns") {
		return "", "", false
	}
	// Every event handler, in one rule rather than a list that can fall behind
	if strings.HasPrefix(lower, "on") {
		return "", "", false
	}

	// xlink:href collapses to href - browsers honour the plain one, and it means a single rule below
	if lower == "href" {
		if !allowedSVGElementAttrs[element]["href"] {
			return "", "", false
		}
		// Same-document fragments ONLY. That is what <use> and gradient inheritance need, and it
		// rules out javascript:, data: and any request that would leave the page
		if !strings.HasPrefix(a.Value, "#") || len(a.Value) < 2 {
			return "", "", false
		}
		return "href", a.Value, true
	}

	if !allowedSVGCommonAttrs[name] && !allowedSVGElementAttrs[element][name] {
		return "", "", false
	}
	if name == "style" {
		clean, ok := sanitizeSVGStyle(a.Value)
		return "style", clean, ok
	}
	// A URL can hide in any paint attribute via url(...), e.g. fill="url(http://evil/x)"
	if v := strings.ToLower(a.Value); strings.Contains(v, "javascript:") || hasExternalURLRef(v) {
		return "", "", false
	}
	return name, a.Value, true
}

// Inline CSS is kept, minus anything that fetches or executes. Declarations are checked one at a
// time so a single bad rule costs its own declaration rather than the element's whole style
func sanitizeSVGStyle(style string) (string, bool) {
	var kept []string
	for decl := range strings.SplitSeq(style, ";") {
		d := strings.TrimSpace(decl)
		if d == "" {
			continue
		}
		low := strings.ToLower(d)
		if strings.Contains(low, "javascript:") || strings.Contains(low, "expression(") ||
			strings.Contains(low, "@import") || hasExternalURLRef(low) {
			continue
		}
		kept = append(kept, d)
	}
	if len(kept) == 0 {
		return "", false
	}
	return strings.Join(kept, ";"), true
}

// url(#gradient) is an internal reference and fine; url(anything-else) reaches off the document, so
// it leaks the viewer's IP at best
func hasExternalURLRef(lowered string) bool {
	rest := lowered
	for {
		i := strings.Index(rest, "url(")
		if i < 0 {
			return false
		}
		arg := strings.TrimLeft(rest[i+len("url("):], " \t'\"")
		if !strings.HasPrefix(arg, "#") {
			return true
		}
		rest = rest[i+len("url("):]
	}
}
