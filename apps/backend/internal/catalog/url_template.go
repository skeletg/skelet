package catalog

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// URL path template engine - one lexing pass where anything outside braces is a literal and anything inside is a token
//   {name} slugified product name, {category} the primary category's full path, {categorySlug} its leaf slug
//   {attr:<slug>} resolves a selectable single value to its slug and a scalar to slugify(stringify(value)); multiselect is rejected at validate time
// Any other token, or a malformed brace, is a parse error

type urlTokenKind int

const (
	urlTokenLiteral urlTokenKind = iota
	urlTokenName
	urlTokenCategory
	urlTokenCategorySlug
	urlTokenAttr
)

// payload is the raw text for literals and the attribute slug for attr tokens, empty for the rest
type urlToken struct {
	Kind    urlTokenKind
	Payload string
}

// No token is required - url_rewrites enforces uniqueness downstream (-1, -2, … on collision), so "{category}/{attr:model}" is valid even though it can collide
func parseURLTemplate(s string) ([]urlToken, error) {
	if strings.TrimSpace(s) == "" {
		return nil, ErrTemplateEmpty
	}
	var tokens []urlToken
	var lit strings.Builder
	flushLit := func() {
		if lit.Len() > 0 {
			tokens = append(tokens, urlToken{Kind: urlTokenLiteral, Payload: lit.String()})
			lit.Reset()
		}
	}
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '{':
			end := strings.IndexByte(s[i+1:], '}')
			if end < 0 {
				return nil, fmt.Errorf("%w: unterminated `{`", ErrTemplateUnbalancedBraces)
			}
			body := s[i+1 : i+1+end]
			if strings.ContainsAny(body, "{") {
				return nil, fmt.Errorf("%w: nested `{` in token %q", ErrTemplateUnbalancedBraces, body)
			}
			flushLit()
			tok, err := classifyToken(body)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = i + 1 + end + 1
		case '}':
			return nil, fmt.Errorf("%w: stray `}`", ErrTemplateUnbalancedBraces)
		default:
			lit.WriteByte(c)
			i++
		}
	}
	flushLit()
	return tokens, nil
}

// Whitespace inside braces is rejected to keep templates unambiguous
func classifyToken(body string) (urlToken, error) {
	if body == "" {
		return urlToken{}, fmt.Errorf("%w: empty `{}`", ErrTemplateUnknownToken)
	}
	if strings.ContainsAny(body, " \t\n") {
		return urlToken{}, fmt.Errorf("%w: token %q contains whitespace", ErrTemplateUnknownToken, body)
	}
	switch body {
	case "name":
		return urlToken{Kind: urlTokenName}, nil
	case "category":
		return urlToken{Kind: urlTokenCategory}, nil
	case "categorySlug":
		return urlToken{Kind: urlTokenCategorySlug}, nil
	}
	if strings.HasPrefix(body, "attr:") {
		slug := body[len("attr:"):]
		if err := validateSlug(slug); err != nil {
			return urlToken{}, fmt.Errorf("%w: attr token %q: %w", ErrTemplateUnknownToken, body, err)
		}
		return urlToken{Kind: urlTokenAttr, Payload: slug}, nil
	}
	return urlToken{}, fmt.Errorf("%w: %q", ErrTemplateUnknownToken, body)
}

// Rejects a multiselect attr token, whose path would change shape per product - pass a ProductType loaded by GetProductTypeWithAttributes so Attributes is populated
func validateURLTemplate(tokens []urlToken, pt ProductType) error {
	if len(pt.Attributes) == 0 && hasAttrToken(tokens) {
		return fmt.Errorf("%w: product type has no attributes", ErrTemplateAttributeNotAssigned)
	}
	bySlug := make(map[string]Attribute, len(pt.Attributes))
	for _, a := range pt.Attributes {
		bySlug[a.Attribute.Slug] = a.Attribute
	}
	for _, t := range tokens {
		if t.Kind != urlTokenAttr {
			continue
		}
		attr, ok := bySlug[t.Payload]
		if !ok {
			return fmt.Errorf("%w: %q", ErrTemplateAttributeNotAssigned, t.Payload)
		}
		if attr.InputType == InputTypeMultiselect {
			return fmt.Errorf("%w: %q", ErrTemplateAttributeMultiselect, attr.Slug)
		}
	}
	return nil
}

func hasAttrToken(tokens []urlToken) bool {
	for _, t := range tokens {
		if t.Kind == urlTokenAttr {
			return true
		}
	}
	return false
}

// attrIndex maps attribute id to Attribute to find the input type for scalars - selectable values already carry their own slug
func renderURLTemplate(
	tokens []urlToken,
	productName string,
	primaryCat *Category,
	attrSelections []ProductAttributeSelection,
	scalars ProductScalarAttributes,
	attrBySlug map[string]Attribute,
) (string, error) {
	selectionsByAttrID := make(map[uuid.UUID][]ProductAttributeSelection, len(attrSelections))
	for _, sel := range attrSelections {
		selectionsByAttrID[sel.AttributeID] = append(selectionsByAttrID[sel.AttributeID], sel)
	}
	var b strings.Builder
	for _, t := range tokens {
		switch t.Kind {
		case urlTokenLiteral:
			b.WriteString(t.Payload)
		case urlTokenName:
			b.WriteString(slugifyScalar(productName))
		case urlTokenCategory:
			if primaryCat == nil {
				return "", ErrTemplateMissingCategory
			}
			b.WriteString(primaryCat.URLPath)
		case urlTokenCategorySlug:
			if primaryCat == nil {
				return "", ErrTemplateMissingCategory
			}
			b.WriteString(primaryCat.Slug)
		case urlTokenAttr:
			attr, ok := attrBySlug[t.Payload]
			if !ok {
				return "", fmt.Errorf("%w: %q", ErrTemplateAttributeNotAssigned, t.Payload)
			}
			value, err := resolveAttrTokenValue(attr, selectionsByAttrID[attr.ID], scalars)
			if err != nil {
				return "", err
			}
			b.WriteString(value)
		}
	}
	out := b.String()
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("%w: rendered to empty string", ErrTemplateEmpty)
	}
	return out, nil
}

func resolveAttrTokenValue(attr Attribute, sels []ProductAttributeSelection, scalars ProductScalarAttributes) (string, error) {
	if attr.InputType.AcceptsValues() {
		if attr.InputType == InputTypeMultiselect {
			return "", fmt.Errorf("%w: %q", ErrTemplateAttributeMultiselect, attr.Slug)
		}
		if len(sels) == 0 {
			return "", fmt.Errorf("%w: %q", ErrTemplateMissingAttribute, attr.Slug)
		}
		return sels[0].Value.Slug, nil
	}
	raw, ok := scalars[attr.Slug]
	if !ok || raw == nil {
		return "", fmt.Errorf("%w: %q", ErrTemplateMissingAttribute, attr.Slug)
	}
	return slugifyScalar(raw), nil
}

// Same algorithm as slugifyForSKU
func slugifyScalar(v any) string {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case bool:
		if x {
			s = "true"
		} else {
			s = "false"
		}
	case float64:
		s = strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", x), "0"), ".")
		if s == "" {
			s = "0"
		}
	default:
		s = fmt.Sprint(v)
	}
	return slugifyForSKU(s)
}
