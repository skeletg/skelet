package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The template decides a product's public url, and url_rewrites only de-duplicates AFTER the fact - so a template that renders the wrong thing does not fail, it quietly ships a wrong (or colliding, then suffixed) path that search engines then index

func attr(slug string, in InputType) Attribute {
	return Attribute{ID: uuid.New(), Slug: slug, Name: slug, InputType: in}
}

func productType(attrs ...Attribute) ProductType {
	pt := ProductType{}
	for _, a := range attrs {
		pt.Attributes = append(pt.Attributes, ProductTypeAttribute{Attribute: a})
	}
	return pt
}

func selection(a Attribute, slug string) ProductAttributeSelection {
	return ProductAttributeSelection{AttributeID: a.ID, Value: AttributeValue{ID: uuid.New(), AttributeID: a.ID, Slug: slug, Name: slug}}
}

func bySlug(attrs ...Attribute) map[string]Attribute {
	m := make(map[string]Attribute, len(attrs))
	for _, a := range attrs {
		m[a.Slug] = a
	}
	return m
}

func mustParse(t *testing.T, tpl string) []urlToken {
	t.Helper()
	tokens, err := parseURLTemplate(tpl)
	if err != nil {
		t.Fatalf("parse %q: %v", tpl, err)
	}
	return tokens
}

func TestParseURLTemplateReadsLiteralsAndTokens(t *testing.T) {
	tokens := mustParse(t, "shop/{category}/{name}.html")
	kinds := make([]urlTokenKind, len(tokens))
	for i, tok := range tokens {
		kinds[i] = tok.Kind
	}
	want := []urlTokenKind{urlTokenLiteral, urlTokenCategory, urlTokenLiteral, urlTokenName, urlTokenLiteral}
	if len(kinds) != len(want) {
		t.Fatalf("token kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("token %d = %v, want %v", i, kinds[i], want[i])
		}
	}
	if tokens[0].Payload != "shop/" || tokens[4].Payload != ".html" {
		t.Errorf("literal payloads = %q / %q, want the text around the tokens", tokens[0].Payload, tokens[4].Payload)
	}
}

func TestParseURLTemplateCarriesTheAttrSlug(t *testing.T) {
	tokens := mustParse(t, "{attr:model}")
	if len(tokens) != 1 || tokens[0].Kind != urlTokenAttr || tokens[0].Payload != "model" {
		t.Fatalf("tokens = %+v, want a single attr token for `model`", tokens)
	}
}

func TestParseURLTemplateRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name, tpl string
		want      error
	}{
		{"empty", "", ErrTemplateEmpty},
		{"whitespace only", "   ", ErrTemplateEmpty},
		{"unterminated brace", "{name", ErrTemplateUnbalancedBraces},
		{"stray closing brace", "name}", ErrTemplateUnbalancedBraces},
		{"nested brace", "{na{me}", ErrTemplateUnbalancedBraces},
		{"empty token", "{}", ErrTemplateUnknownToken},
		{"unknown token", "{sku}", ErrTemplateUnknownToken},
		// whitespace inside braces would make `{ name }` and `{name}` two spellings of one thing
		{"whitespace in token", "{ name }", ErrTemplateUnknownToken},
		{"attr with no slug", "{attr:}", ErrTemplateUnknownToken},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseURLTemplate(c.tpl); !errors.Is(err, c.want) {
				t.Errorf("parse %q err = %v, want %v", c.tpl, err, c.want)
			}
		})
	}
}

func TestValidateURLTemplateRejectsAttributesItCannotResolve(t *testing.T) {
	model := attr("model", InputTypeDropdown)
	multi := attr("tags", InputTypeMultiselect)

	if err := validateURLTemplate(mustParse(t, "{attr:model}"), productType(model)); err != nil {
		t.Errorf("assigned single-select attr rejected: %v", err)
	}
	if err := validateURLTemplate(mustParse(t, "{attr:missing}"), productType(model)); !errors.Is(err, ErrTemplateAttributeNotAssigned) {
		t.Errorf("err = %v, want ErrTemplateAttributeNotAssigned for an attribute the type does not carry", err)
	}
	// a multiselect would give the path a different shape per product, so it never gets that far
	if err := validateURLTemplate(mustParse(t, "{attr:tags}"), productType(multi)); !errors.Is(err, ErrTemplateAttributeMultiselect) {
		t.Errorf("err = %v, want ErrTemplateAttributeMultiselect", err)
	}
	if err := validateURLTemplate(mustParse(t, "{attr:model}"), productType()); !errors.Is(err, ErrTemplateAttributeNotAssigned) {
		t.Errorf("err = %v, want ErrTemplateAttributeNotAssigned when the type has no attributes at all", err)
	}
	// a template with no attr token cares about none of this
	if err := validateURLTemplate(mustParse(t, "{name}"), productType()); err != nil {
		t.Errorf("attribute-free template rejected: %v", err)
	}
}

func TestRenderURLTemplateSlugifiesTheName(t *testing.T) {
	got, err := renderURLTemplate(mustParse(t, "p/{name}"), "Chelsea Boot  MkII", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.ContainsAny(got, " ") || !strings.HasPrefix(got, "p/") {
		t.Errorf("rendered %q, want the name slugified under the literal prefix", got)
	}
}

func TestRenderURLTemplateUsesTheCategoryPathAndLeaf(t *testing.T) {
	cat := &Category{Slug: "running", URLPath: "shoes/mens/running"}
	full, err := renderURLTemplate(mustParse(t, "{category}/x"), "n", cat, nil, nil, nil)
	if err != nil || full != "shoes/mens/running/x" {
		t.Errorf("{category} rendered %q (%v), want the full hierarchical path", full, err)
	}
	leaf, err := renderURLTemplate(mustParse(t, "{categorySlug}/x"), "n", cat, nil, nil, nil)
	if err != nil || leaf != "running/x" {
		t.Errorf("{categorySlug} rendered %q (%v), want just the leaf", leaf, err)
	}
}

func TestRenderURLTemplateNeedsACategoryWhenItAsksForOne(t *testing.T) {
	for _, tpl := range []string{"{category}", "{categorySlug}"} {
		if _, err := renderURLTemplate(mustParse(t, tpl), "n", nil, nil, nil, nil); !errors.Is(err, ErrTemplateMissingCategory) {
			t.Errorf("%s with no primary category: err = %v, want ErrTemplateMissingCategory", tpl, err)
		}
	}
}

func TestRenderURLTemplateResolvesAttributes(t *testing.T) {
	model := attr("model", InputTypeDropdown)
	sels := []ProductAttributeSelection{selection(model, "mk-ii")}

	got, err := renderURLTemplate(mustParse(t, "{attr:model}"), "n", nil, sels, nil, bySlug(model))
	if err != nil || got != "mk-ii" {
		t.Errorf("selected value rendered %q (%v), want its slug", got, err)
	}

	// a scalar has no value row to carry a slug, so it gets slugified on the way out
	size := attr("size", InputTypePlainText)
	got, err = renderURLTemplate(mustParse(t, "{attr:size}"), "n", nil, nil, ProductScalarAttributes{"size": "Extra Large"}, bySlug(size))
	if err != nil || got != "extra-large" {
		t.Errorf("scalar rendered %q (%v), want it slugified", got, err)
	}
}

func TestRenderURLTemplateRefusesAnAttributeTheProductHasNotSet(t *testing.T) {
	model := attr("model", InputTypeDropdown)
	if _, err := renderURLTemplate(mustParse(t, "{attr:model}"), "n", nil, nil, nil, bySlug(model)); !errors.Is(err, ErrTemplateMissingAttribute) {
		t.Errorf("err = %v, want ErrTemplateMissingAttribute when nothing is selected", err)
	}
	size := attr("size", InputTypePlainText)
	if _, err := renderURLTemplate(mustParse(t, "{attr:size}"), "n", nil, nil, ProductScalarAttributes{}, bySlug(size)); !errors.Is(err, ErrTemplateMissingAttribute) {
		t.Errorf("err = %v, want ErrTemplateMissingAttribute for an absent scalar", err)
	}
	// an explicit null is just as absent - rendering "<nil>" into a url would be worse than failing
	if _, err := renderURLTemplate(mustParse(t, "{attr:size}"), "n", nil, nil, ProductScalarAttributes{"size": nil}, bySlug(size)); !errors.Is(err, ErrTemplateMissingAttribute) {
		t.Errorf("err = %v, want ErrTemplateMissingAttribute for a null scalar", err)
	}
	if _, err := renderURLTemplate(mustParse(t, "{attr:gone}"), "n", nil, nil, nil, bySlug()); !errors.Is(err, ErrTemplateAttributeNotAssigned) {
		t.Errorf("err = %v, want ErrTemplateAttributeNotAssigned for an unknown attribute", err)
	}
}

func TestRenderURLTemplateRefusesToProduceAnEmptyPath(t *testing.T) {
	// every token can legitimately render nothing, and "" is not a url anyone can visit
	if _, err := renderURLTemplate(mustParse(t, "{name}"), "   ", nil, nil, nil, nil); !errors.Is(err, ErrTemplateEmpty) {
		t.Errorf("err = %v, want ErrTemplateEmpty when the whole template renders blank", err)
	}
}

func TestSlugifyScalarHandlesEveryJSONShape(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"string", "Extra Large", "extra-large"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"whole number keeps no decimals", float64(42), "42"},
		{"fraction keeps its digits", 1.5, "1-5"},
		{"zero", float64(0), "0"},
		{"integer", 7, "7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := slugifyScalar(c.in); got != c.want {
				t.Errorf("slugifyScalar(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Update always validated the template, create never did - a type created with an unknown token then failed
// every save, a rename included, on a template the admin wasn't touching
func TestCreateProductTypeRejectsATemplateUpdateWouldRefuse(t *testing.T) {
	var s core // validation runs before the repository is reached
	create := func(tpl string) error {
		_, err := s.CreateProductType(context.Background(), CreateProductTypeInput{Slug: "tpl-check", Name: "Template check", URLTemplate: tpl})
		return err
	}
	if err := create("e2e/{slug}"); !errors.Is(err, ErrTemplateUnknownToken) {
		t.Errorf("unknown token: got %v, want ErrTemplateUnknownToken", err)
	}
	// A new type has no attributes, so an attribute token can't resolve yet
	if err := create("{attr:colour}"); err == nil {
		t.Error("attribute token on a type with no attributes: got nil, want a refusal")
	}
}
