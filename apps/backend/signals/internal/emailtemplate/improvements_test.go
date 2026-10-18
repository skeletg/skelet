package emailtemplate

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestRenderStoredOrderConfirmation locks the flow-engine render path: a specific (flow-chosen) library email renders
// with the full order context, so the order_items / order_totals / shipping_address data blocks populate
func TestRenderStoredOrderConfirmation(t *testing.T) {
	svc := NewService(fakeRepo{}, nil)
	e := LibraryEmail{
		ChannelID: uuid.New(),
		Slot:      KeyOrderConfirmation,
		Subject:   "Order {{order.number}} confirmed",
		Blocks: []Block{
			{Type: "heading", Text: "Thanks {{customer.firstName}}"},
			{Type: "order_items"},
			{Type: "order_totals"},
			{Type: "shipping_address"},
		},
	}
	subj, html, _, err := svc.RenderStoredOrderConfirmation(context.Background(), e.ChannelID, e, sampleConfirmation())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if subj != "Order ORD-9 confirmed" {
		t.Errorf("subject not interpolated: %q", subj)
	}
	for _, want := range []string{"Thanks Sam", "Tee", "T1", "26.60 USD", "Sam Doe"} {
		if !strings.Contains(html, want) {
			t.Errorf("order-context render missing %q in:\n%s", want, html)
		}
	}
}

// TestScriptedBlockLiquid locks the Liquid render path: a scripted reusable block renders conditionals + loops over the
// order data, injects sanitized custom CSS, and still strips active content
func TestScriptedBlockLiquid(t *testing.T) {
	repo := fakeRepo{blocks: map[string]Snippet{
		"promo": {
			Slug: "promo", Name: "Promo", Scripted: true,
			CSS: `.promo{color:red} @import url('evil.css'); a{background:expression(alert(1))}`,
			HTMLTemplate: `<div class="promo">` +
				`{% if customer.firstName %}Hi {{ customer.firstName }}!{% endif %}` +
				`<ul>{% for item in order.items %}<li>{{ item.name }} x{{ item.quantity }} = {{ item.line_total }}</li>{% endfor %}</ul>` +
				`<script>alert(1)</script></div>`,
		},
	}}
	svc := NewService(repo, nil)
	e := LibraryEmail{ChannelID: uuid.New(), Subject: "S", Blocks: []Block{{Type: "ref", Slug: "promo"}}}
	_, html, _, err := svc.RenderStoredOrderConfirmation(context.Background(), e.ChannelID, e, sampleConfirmation())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"Hi Sam!", "Tee x1", "20.00 USD", "<style>", ".promo"} {
		if !strings.Contains(html, want) {
			t.Errorf("scripted render missing %q in:\n%s", want, html)
		}
	}
	if strings.Contains(html, "<script") {
		t.Errorf("script not stripped:\n%s", html)
	}
	if strings.Contains(html, "@import") {
		t.Errorf("CSS @import not neutralized:\n%s", html)
	}
	if strings.Contains(strings.ToLower(html), "expression(") {
		t.Errorf("CSS expression() not neutralized:\n%s", html)
	}
}

func TestInterpolateFallback(t *testing.T) {
	vars := map[string]string{"customer.firstName": "Sam"}
	cases := map[string]string{
		`Hi {{customer.firstName | default: "there"}}`: "Hi Sam",
		`Hi {{customer.lastName | default: "there"}}`:  "Hi there",
		`Hi {{customer.lastName | friend}}`:            "Hi friend", // shorthand
		`Hi {{customer.lastName}}`:                     "Hi ",       // no fallback → empty
	}
	for in, want := range cases {
		if got := interpolate(in, vars); got != want {
			t.Errorf("interpolate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBlockConditionEval(t *testing.T) {
	vars := map[string]string{"customer.group": "vip", "order.number": "ORD-1"}
	cases := []struct {
		c    BlockCondition
		want bool
	}{
		{BlockCondition{Field: "customer.group", Op: "eq", Value: "vip"}, true},
		{BlockCondition{Field: "customer.group", Op: "eq", Value: "retail"}, false},
		{BlockCondition{Field: "customer.group", Op: "ne", Value: "retail"}, true},
		{BlockCondition{Field: "order.number", Op: "exists"}, true},
		{BlockCondition{Field: "missing", Op: "exists"}, false},
		{BlockCondition{Field: "customer.group", Op: "eq", Value: "vip", Negate: true}, false},
		{BlockCondition{Field: "customer.group", Op: "contains", Value: "IP"}, true},
	}
	for _, c := range cases {
		if got := c.c.eval(vars); got != c.want {
			t.Errorf("eval(%+v) = %v, want %v", c.c, got, c.want)
		}
	}
}

// A hidden block must not appear in the rendered output; a shown one must
func TestRenderSkipsHiddenBlocks(t *testing.T) {
	svc := NewService(fakeRepo{}, nil)
	blocks := []Block{
		{Type: "heading", Text: "Always here"},
		{Type: "heading", Text: "VIP only", VisibleIf: &BlockCondition{Field: "customer.group", Op: "eq", Value: "vip"}},
	}
	// Non-VIP → the gated block is dropped
	_, html, _, err := svc.RenderContent(context.Background(), uuid.New(), "S", "", blocks, EmailStyles{}, map[string]string{"customer.group": "retail"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, "VIP only") {
		t.Error("gated block should be hidden for a non-matching recipient")
	}
	if !strings.Contains(html, "Always here") {
		t.Error("ungated block should always render")
	}
	// VIP → the gated block appears
	_, html, _, _ = svc.RenderContent(context.Background(), uuid.New(), "S", "", blocks, EmailStyles{}, map[string]string{"customer.group": "vip"})
	if !strings.Contains(html, "VIP only") {
		t.Error("gated block should render for a matching recipient")
	}
}

func TestPreheaderInjected(t *testing.T) {
	svc := NewService(fakeRepo{}, nil)
	_, html, _, err := svc.RenderContent(context.Background(), uuid.New(), "Subject", "Peek inside {{customer.firstName}}", nil, EmailStyles{}, map[string]string{"customer.firstName": "Sam"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(html, "Peek inside Sam") {
		t.Errorf("preheader (interpolated) should be injected:\n%s", html)
	}
	if !strings.Contains(html, "display:none") {
		t.Error("preheader should be in a hidden span")
	}
}

func TestPreflight(t *testing.T) {
	warns := Preflight("", "", []Block{
		{Type: "image", URL: "https://x/y.png"}, // missing alt
		{Type: "button", Label: "Go"},           // missing link
	})
	var msgs strings.Builder
	for _, w := range warns {
		msgs.WriteString(w.Message + "\n")
	}
	for _, want := range []string{"No subject", "No preheader", "alt text", "no link"} {
		if !strings.Contains(msgs.String(), want) {
			t.Errorf("preflight missing %q; got:\n%s", want, msgs.String())
		}
	}
	// A clean email → no missing-subject/alt/link warnings
	clean := Preflight("Hello", "Peek", []Block{
		{Type: "heading", Text: "Hi"},
		{Type: "image", URL: "https://x/y.png", Alt: "product"},
		{Type: "button", Label: "Shop", URL: "https://x/shop"},
	})
	for _, w := range clean {
		if w.Severity == "warn" {
			t.Errorf("clean email should have no warnings, got: %s", w.Message)
		}
	}
}
