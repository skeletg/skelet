package emailtemplate

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/email"
)

func TestInterpolate(t *testing.T) {
	vars := map[string]string{"order.number": "ORD-1", "store.name": "Acme"}
	cases := map[string]string{
		"Order {{order.number}} from {{store.name}}": "Order ORD-1 from Acme",
		"{{unknown.token}}":                          "",
		"no tokens here":                             "no tokens here",
		"unterminated {{order.number":                "unterminated {{order.number",
		"{{ order.number }}":                         "ORD-1", // trimmed
	}
	for in, want := range cases {
		if got := interpolate(in, vars); got != want {
			t.Errorf("interpolate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateBlock(t *testing.T) {
	for _, ty := range []string{"heading", "text", "button", "image", "divider", "spacer", "order_items", "order_totals", "shipping_address", "tracking", "refund_amount"} {
		if err := validateBlock(Block{Type: ty}, true); err != nil {
			t.Errorf("validateBlock(%s) errored: %v", ty, err)
		}
	}
	// Refs are valid regardless of nesting now (render-time cycle/depth guard);
	// they only require a slug
	if err := validateBlock(Block{Type: "ref", Slug: "x"}, true); err != nil {
		t.Errorf("ref with slug should be valid: %v", err)
	}
	if err := validateBlock(Block{Type: "ref", Slug: "x"}, false); err != nil {
		t.Errorf("ref is now allowed inside snippets (nesting): %v", err)
	}
	if err := validateBlock(Block{Type: "ref"}, true); err == nil {
		t.Error("ref without slug should error")
	}
	// Unknown types are treated as custom block-type instances (resolved at
	// render time), so validation accepts them
	if err := validateBlock(Block{Type: "my_custom_type"}, true); err != nil {
		t.Errorf("custom type should be accepted at validation: %v", err)
	}
}

func TestBlockRendersEscapeAndInterpolate(t *testing.T) {
	rc := renderCtx{Vars: map[string]string{"customer.firstName": "<Alex>"}}
	html := blockHTML["heading"](Block{Type: "heading", Text: "Hi {{customer.firstName}}"}, rc)
	if !strings.Contains(html, "Hi &lt;Alex&gt;") {
		t.Errorf("heading should interpolate + HTML-escape: %q", html)
	}
	txt := blockText["heading"](Block{Type: "heading", Text: "Hi {{customer.firstName}}"}, rc)
	if strings.TrimSpace(txt) != "Hi <Alex>" {
		t.Errorf("heading text should interpolate (no escaping): %q", txt)
	}
}

// fakeRepo backs service tests without a DB
type fakeRepo struct {
	templates map[string]Template // key -> global template
	blocks    map[string]Snippet  // slug -> reusable block (component)
	library   map[uuid.UUID]LibraryEmail
	slots     map[string]LibraryEmail // slot -> assigned email (per this fake's single channel)
}

func (f fakeRepo) Get(_ context.Context, key string, channelID *uuid.UUID) (Template, bool, error) {
	if channelID != nil {
		return Template{}, false, nil // no channel overrides in these tests
	}
	t, ok := f.templates[key]
	return t, ok, nil
}
func (f fakeRepo) List(context.Context) ([]Template, error)         { return nil, nil }
func (f fakeRepo) Save(context.Context, Template) error             { return nil }
func (f fakeRepo) Delete(context.Context, string, *uuid.UUID) error { return nil }
func (f fakeRepo) GetBlock(_ context.Context, _ uuid.UUID, slug string) (Snippet, bool, error) {
	s, ok := f.blocks[slug]
	return s, ok, nil
}
func (f fakeRepo) ListBlocks(context.Context, uuid.UUID) ([]Snippet, error) { return nil, nil }
func (f fakeRepo) SaveBlock(context.Context, Snippet) error                 { return nil }
func (f fakeRepo) DeleteBlock(context.Context, uuid.UUID, string) error     { return nil }

func (f fakeRepo) CreateLibraryEmail(_ context.Context, e LibraryEmail) (LibraryEmail, error) {
	return e, nil
}
func (f fakeRepo) GetLibraryEmail(_ context.Context, id uuid.UUID) (LibraryEmail, bool, error) {
	e, ok := f.library[id]
	return e, ok, nil
}
func (f fakeRepo) ListLibraryEmails(context.Context, uuid.UUID, string) ([]LibraryEmail, error) {
	return nil, nil
}
func (f fakeRepo) GetSlotEmail(_ context.Context, _ uuid.UUID, slot string) (LibraryEmail, bool, error) {
	e, ok := f.slots[slot]
	return e, ok, nil
}
func (f fakeRepo) UpdateLibraryEmail(context.Context, LibraryEmail) (bool, error) { return true, nil }
func (f fakeRepo) ClearSlot(context.Context, uuid.UUID, string) error             { return nil }
func (f fakeRepo) AssignSlot(context.Context, uuid.UUID, string) error            { return nil }
func (f fakeRepo) DeleteLibraryEmail(context.Context, uuid.UUID) error            { return nil }
func (f fakeRepo) ListEmailGroups(context.Context) ([]EmailGroup, error)          { return nil, nil }
func (f fakeRepo) GetEmailGroup(context.Context, uuid.UUID) (EmailGroup, error) {
	return EmailGroup{}, nil
}
func (f fakeRepo) CreateEmailGroup(context.Context, string) (EmailGroup, error) {
	return EmailGroup{}, nil
}
func (f fakeRepo) RenameEmailGroup(context.Context, uuid.UUID, string) (EmailGroup, error) {
	return EmailGroup{}, nil
}
func (f fakeRepo) DeleteEmailGroup(context.Context, uuid.UUID) error { return nil }
func (f fakeRepo) CountEmailsInGroup(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func (f fakeRepo) InsertRevision(context.Context, LibraryEmail, *uuid.UUID, int) error {
	return nil
}
func (f fakeRepo) ListRevisions(context.Context, uuid.UUID, int32) ([]EmailRevision, error) {
	return nil, nil
}
func (f fakeRepo) GetRevision(context.Context, uuid.UUID) (EmailRevision, bool, error) {
	return EmailRevision{}, false, nil
}

func sampleConfirmation() email.OrderConfirmationData {
	return email.OrderConfirmationData{
		StoreName: "Acme", OrderNumber: "ORD-9", FirstName: "Sam", Currency: "USD",
		Lines:    []email.OrderEmailLine{{Name: "Tee", SKU: "T1", Quantity: 1, UnitPrice: 2000, LineTotal: 2000}},
		Subtotal: 2000, Shipping: 500, Tax: 160, Total: 2660,
		ShipTo: []string{"Sam Doe", "1 St", "US"},
	}
}

func TestServiceFallbackMatchesBuiltin(t *testing.T) {
	svc := NewService(fakeRepo{}, nil, nil) // empty repo → no rows → built-in
	d := sampleConfirmation()
	subj, html, text, err := svc.RenderOrderConfirmation(context.Background(), uuid.Nil, d)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	wSubj, wHTML, wText, _ := email.RenderOrderConfirmation(d)
	if subj != wSubj || html != wHTML || text != wText {
		t.Error("fallback output must be byte-identical to the built-in renderer")
	}
}

func TestSlotEmailOverridesBuiltin(t *testing.T) {
	ch := uuid.New()
	d := sampleConfirmation()

	// No slot email assigned (but a real channel) → still byte-identical built-in
	svc := NewService(fakeRepo{}, nil, nil)
	_, html, _, err := svc.RenderOrderConfirmation(context.Background(), ch, d)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	_, wHTML, _, _ := email.RenderOrderConfirmation(d)
	if html != wHTML {
		t.Error("with no slot email, a real channel must still render the byte-identical built-in")
	}

	// Assign an email to the order_confirmation slot → it wins over the built-in
	svc = NewService(fakeRepo{slots: map[string]LibraryEmail{
		KeyOrderConfirmation: {Name: "Custom confirm", Subject: "Your order {{order.number}}", Blocks: []Block{{Type: "heading", Text: "Thanks for your order!"}}},
	}}, nil, nil)
	_, html, _, err = svc.RenderOrderConfirmation(context.Background(), ch, d)
	if err != nil {
		t.Fatalf("render with slot email: %v", err)
	}
	if !strings.Contains(html, "Thanks for your order!") {
		t.Errorf("slot-assigned email should render its own content:\n%s", html)
	}
}

func TestRenderLibraryEmail(t *testing.T) {
	id := uuid.New()
	svc := NewService(fakeRepo{library: map[uuid.UUID]LibraryEmail{
		id: {ID: id, Name: "Welcome", Subject: "Hi {{customer.firstName}}", Blocks: []Block{{Type: "heading", Text: "Welcome {{customer.firstName}}!"}}},
	}}, nil, nil)
	subj, html, _, err := svc.RenderLibraryEmail(context.Background(), id, map[string]string{"customer.firstName": "Sam"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if subj != "Hi Sam" {
		t.Errorf("subject = %q", subj)
	}
	if !strings.Contains(html, "Welcome Sam!") {
		t.Errorf("html missing interpolated heading:\n%s", html)
	}
	if _, _, _, err := svc.RenderLibraryEmail(context.Background(), uuid.New(), nil); err == nil {
		t.Error("expected error for missing library email")
	}
}

func TestRenderCustomTypeAndSanitize(t *testing.T) {
	// A parameterized HTML-body block (the former "block type" - now just a reusable block with fields + an html_template) Slug must not
	// collide with a built-in block type (e.g. "countdown"), so the {type:<slug>} alias form resolves to this custom snippet
	repo := fakeRepo{blocks: map[string]Snippet{
		"promohtml": {
			Slug: "promohtml", Name: "Promo HTML",
			Fields:       []FieldDef{{Key: "label", Kind: "text"}},
			HTMLTemplate: `<div class="cd">{{label}} for {{customer.firstName}}<script>alert(1)</script><a href="javascript:x">go</a></div>`,
		},
	}}
	svc := NewService(repo, nil, nil)
	vars := map[string]string{"customer.firstName": "Sam"}
	// Both the ref form and the {type:<slug>} alias form resolve the same
	for _, b := range []Block{
		{Type: "ref", Slug: "promohtml", Fields: map[string]string{"label": "<Sale>"}},
		{Type: "promohtml", Fields: map[string]string{"label": "<Sale>"}},
	} {
		_, html, _, err := svc.RenderContent(context.Background(), uuid.New(), "S", "", []Block{b}, EmailStyles{}, vars)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if !strings.Contains(html, `class="cd"`) {
			t.Errorf("template HTML missing:\n%s", html)
		}
		if !strings.Contains(html, "&lt;Sale&gt;") {
			t.Errorf("field value not HTML-escaped:\n%s", html)
		}
		if !strings.Contains(html, "for Sam") {
			t.Errorf("merge tag not interpolated:\n%s", html)
		}
		if strings.Contains(html, "<script") {
			t.Errorf("script tag not sanitized:\n%s", html)
		}
		if strings.Contains(html, "javascript:") {
			t.Errorf("javascript: URL not neutralized:\n%s", html)
		}
	}
	// An unknown block (no matching slug) renders nothing, not an error
	_, html2, _, err := svc.RenderContent(context.Background(), uuid.New(), "S", "", []Block{{Type: "nope"}}, EmailStyles{}, nil)
	if err != nil || strings.Contains(html2, "nope") {
		t.Errorf("unknown type should render empty: err=%v html=%s", err, html2)
	}
}

func TestRenderParameterizedCanvasBlock(t *testing.T) {
	// A reusable block with fields + a COMPOSED body that uses {{field}}
	// placeholders - the new "parameterized canvas block" capability
	repo := fakeRepo{blocks: map[string]Snippet{
		"hero": {
			Slug: "hero", Name: "Hero",
			Fields: []FieldDef{{Key: "title", Kind: "text"}},
			Blocks: []Block{{Type: "heading", Text: "{{title}} - {{customer.firstName}}"}},
		},
	}}
	svc := NewService(repo, nil, nil)
	blocks := []Block{{Type: "ref", Slug: "hero", Fields: map[string]string{"title": "Big Sale"}}}
	_, html, _, err := svc.RenderContent(context.Background(), uuid.New(), "S", "", blocks, EmailStyles{}, map[string]string{"customer.firstName": "Sam"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(html, "Big Sale - Sam") {
		t.Errorf("field value + merge tag should interpolate into the composed body:\n%s", html)
	}
}

func TestBlockNestingAndCycleGuard(t *testing.T) {
	repo := fakeRepo{blocks: map[string]Snippet{
		"a": {Slug: "a", Blocks: []Block{{Type: "text", Content: "AAA"}, {Type: "ref", Slug: "b"}}},
		"b": {Slug: "b", Blocks: []Block{{Type: "text", Content: "BBB"}, {Type: "ref", Slug: "a"}}}, // a→b→a cycle
	}}
	svc := NewService(repo, nil, nil)
	_, html, _, err := svc.RenderContent(context.Background(), uuid.New(), "S", "", []Block{{Type: "ref", Slug: "a"}}, EmailStyles{}, nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Both nested blocks render; the cycle (a→b→a) is stopped (finite output)
	if !strings.Contains(html, "AAA") || !strings.Contains(html, "BBB") {
		t.Errorf("expected nested A+B content:\n%s", html)
	}
}

func TestValidateStyleAndColor(t *testing.T) {
	// valid colors
	for _, c := range []string{"", "#fff", "#ffffff", "#ffffff80", "rgb(1,2,3)", "rgba(1,2,3,0.5)", "red", "TRANSPARENT"} {
		if err := validateColor(c); err != nil {
			t.Errorf("validateColor(%q) should pass: %v", c, err)
		}
	}
	// injection / bad colors rejected
	for _, c := range []string{"red;}", `#fff" onload="x`, "url(javascript:1)", "expression(1)"} {
		if err := validateColor(c); err == nil {
			t.Errorf("validateColor(%q) should be rejected", c)
		}
	}
	if err := validateStyle(&BlockStyle{Align: "diagonal"}); err == nil {
		t.Error("bad align should be rejected")
	}
	if err := validateStyle(&BlockStyle{FontSize: 9999}); err == nil {
		t.Error("out-of-range fontSize should be rejected")
	}
	if err := validateStyle(&BlockStyle{Align: "center", Color: "#abc", PaddingY: 12, Radius: 6}); err != nil {
		t.Errorf("valid style rejected: %v", err)
	}
	if err := validateStyles(EmailStyles{ContentWidth: 50}); err == nil {
		t.Error("too-small contentWidth should be rejected")
	}
	if err := validateStyles(EmailStyles{FontFamily: "Arial</style>"}); err == nil {
		t.Error("font family with markup should be rejected")
	}
}

func TestValidateRowBlock(t *testing.T) {
	row := Block{Type: "row", Columns: []Column{
		{Width: 1, Blocks: []Block{{Type: "image", URL: "x"}}},
		{Width: 2, Blocks: []Block{{Type: "text", Content: "hi"}}},
	}}
	if err := validateBlock(row, true); err != nil {
		t.Errorf("valid row rejected: %v", err)
	}
	// no nested rows
	nested := Block{Type: "row", Columns: []Column{{Blocks: []Block{{Type: "row"}}}}}
	if err := validateBlock(nested, true); err == nil {
		t.Error("nested row should be rejected")
	}
	// no ref inside a column
	withRef := Block{Type: "row", Columns: []Column{{Blocks: []Block{{Type: "ref", Slug: "x"}}}}}
	if err := validateBlock(withRef, true); err == nil {
		t.Error("ref inside a column should be rejected")
	}
	if err := validateBlock(Block{Type: "row"}, true); err == nil {
		t.Error("row with no columns should be rejected")
	}
}

func TestRenderStyledAndRow(t *testing.T) {
	repo := fakeRepo{
		templates: map[string]Template{
			KeyOrderConfirmation: {
				Key:     KeyOrderConfirmation,
				Subject: "S",
				Styles:  EmailStyles{Background: "#101010", ContentWidth: 700, LinkColor: "#0a0"},
				Blocks: []Block{
					{Type: "heading", Text: "Hi", Style: &BlockStyle{Align: "center", Color: "#ff0000", FontSize: 28}},
					{Type: "row", Columns: []Column{
						{Width: 1, Blocks: []Block{{Type: "text", Content: "left"}}},
						{Width: 1, Blocks: []Block{{Type: "text", Content: "right"}}},
					}},
				},
			},
		},
	}
	svc := NewService(repo, nil, nil)
	_, html, text, err := svc.RenderOrderConfirmation(context.Background(), uuid.Nil, sampleConfirmation())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"background:#101010",  // theme background
		"max-width:700px",     // theme content width
		"a{color:#0a0;}",      // theme link color
		"text-align:center",   // heading align override
		"color:#ff0000",       // heading color override
		"font-size:28px",      // heading font-size override
		`<table width="100%"`, // row table
		"width:50%",           // two equal columns
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q:\n%s", want, html)
		}
	}
	// row columns contribute to the text part too
	if !strings.Contains(text, "left") || !strings.Contains(text, "right") {
		t.Errorf("text missing column content: %q", text)
	}
}

func TestServiceRendersTemplateAndRef(t *testing.T) {
	repo := fakeRepo{
		templates: map[string]Template{
			KeyOrderConfirmation: {Key: KeyOrderConfirmation, Subject: "Your {{store.name}} order {{order.number}}", Blocks: []Block{
				{Type: "heading", Text: "Thanks {{customer.firstName}}"},
				{Type: "order_items"},
				{Type: "ref", Slug: "promo"},
			}},
		},
		blocks: map[string]Snippet{
			"promo": {Slug: "promo", Blocks: []Block{{Type: "text", Content: "Come back soon!"}}},
		},
	}
	svc := NewService(repo, nil, nil)
	subj, html, _, err := svc.RenderOrderConfirmation(context.Background(), uuid.Nil, sampleConfirmation())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if subj != "Your Acme order ORD-9" {
		t.Errorf("subject = %q", subj)
	}
	for _, want := range []string{"Thanks Sam", "Tee", "Come back soon!", "<!DOCTYPE html>"} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q:\n%s", want, html)
		}
	}
}
