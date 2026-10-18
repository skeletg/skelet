package emailtemplate

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func scripted(html string) Snippet {
	return Snippet{ChannelID: uuid.New(), Slug: "s", Name: "S", Scripted: true, HTMLTemplate: html}
}

// A scripted block is operator-authored code running in the send loop, so a save has to prove it renders cheaply
func TestCostlyScriptedBlocksAreRefusedAtSave(t *testing.T) {
	svc := NewService(fakeRepo{}, nil)
	for name, html := range map[string]string{
		"a huge literal loop":        `{% for i in (1..100000000) %}x{% endfor %}`,
		"output past the cap":        `{% for i in (1..999) %}` + strings.Repeat("y", 1024) + `{% endfor %}`,
		"a broken template":          `{% if %}`,
		"loops past the time budget": `{% for i in (1..999) %}{% for j in (1..999) %}{% assign k = j | plus: i %}{% endfor %}{% endfor %}`,
		"a self-feeding capture":     `{% capture s %}x{% endcapture %}`,
	} {
		if err := svc.SaveBlock(context.Background(), scripted(html)); err == nil {
			t.Errorf("%s: saved", name)
		}
	}
	if err := svc.SaveBlock(context.Background(), scripted(`{% for i in (1..5000) %}x{% endfor %}`)); err == nil || !strings.Contains(err.Error(), "loops over") {
		t.Errorf("a literal loop past the range cap: err = %v, want it refused before rendering", err)
	}
	ok := `{% for item in order.items %}<li>{{ item.name }}</li>{% endfor %}{% for i in (1..10) %}.{% endfor %}`
	if err := svc.SaveBlock(context.Background(), scripted(ok)); err != nil {
		t.Errorf("an ordinary scripted block was refused: %v", err)
	}
}

func TestStringGrowthIsCapped(t *testing.T) {
	doubling := `{% assign s = "xxxxxxxx" %}{% for i in (1..16) %}{% assign s = s | append: s %}{% endfor %}{{ s | size }}`
	out, _, err := renderLiquid(doubling, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "65536" {
		t.Errorf("a doubling append grew to %s bytes, want the %d cap", out, maxScriptedString)
	}
	blowup := `{% assign s = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" %}{{ s | replace: "a", "` + strings.Repeat("b", 2000) + `" | size }}`
	if out, _, err = renderLiquid(blowup, map[string]any{}); err != nil || strings.TrimSpace(out) == "100000" {
		t.Errorf("replace multiplied a string past the cap: %s, %v", out, err)
	}
}

func TestAScriptedErrorStaysOutOfTheCustomersMail(t *testing.T) {
	svc := NewService(fakeRepo{}, nil)
	html, _ := svc.renderScriptedHTML(scripted(`Hello {{ customer.firstName }}{% if %}`), map[string]string{"customer.firstName": "Sam"}, renderCtx{})
	if strings.Contains(html, "template error") || strings.Contains(html, "<!--") {
		t.Errorf("the template's error reached the email: %s", html)
	}
}

// Every link a block carries is a link in a customer's inbox - only web, mail and phone ones, or a merge tag that becomes one
func TestBlockLinksMustBeSafeSchemes(t *testing.T) {
	for _, bad := range []Block{
		{Type: "button", Label: "Go", URL: "javascript:alert(1)"},
		{Type: "image", URL: "data:text/html,<script>"},
		{Type: "video", URL: "https://v.example/1", Poster: "vbscript:x"},
		{Type: "social", Social: []SocialLink{{Platform: "x", URL: "javascript:void(0)"}}},
		{Type: "row", Columns: []Column{{Width: 1, Blocks: []Block{{Type: "button", URL: "javascript:alert(1)"}}}}},
	} {
		if err := validateBlock(bad, true); err == nil {
			t.Errorf("%s block with %+v saved", bad.Type, bad)
		}
	}
	for _, good := range []string{"https://shop.example/sale", "mailto:help@shop.example", "tel:+15550100", "{{storefront.url}}/account", ""} {
		if err := validateBlock(Block{Type: "button", Label: "Go", URL: good}, true); err != nil {
			t.Errorf("button to %q refused: %v", good, err)
		}
	}
}
