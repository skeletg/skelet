package emailtemplate

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// What these pin: only the subset survives, merge values stay text, and an older plain block renders as it did
func TestRichTextKeepsItsFormattingAndNothingElse(t *testing.T) {
	svc := NewService(fakeRepo{}, nil)
	content := `<p>Hi <strong>{{customer.firstName}}</strong>, <em>thanks</em> for <u>shopping</u>.</p>` +
		`<ul><li>Fast shipping</li><li><a href="{{storefront.url}}/sale">The sale</a></li></ul>` +
		`<p onclick="steal()" style="font-size:90px">Clean <script>alert(1)</script><a href="javascript:alert(1)">bad link</a></p>`
	vars := map[string]string{"customer.firstName": `<b>Ada</b>`, "storefront.url": "https://shop.test"}
	_, htmlOut, text, err := svc.RenderContent(context.Background(), uuid.Nil, "Hi", "", []Block{{Type: "text", Format: FormatRich, Content: content}},
		EmailStyles{LinkColor: "#0055ff"}, vars)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<strong>&lt;b&gt;Ada&lt;/b&gt;</strong>`,
		`<em>thanks</em>`, `<u>shopping</u>`,
		`<ul style="margin:0 0 12px;padding-left:20px;"><li>Fast shipping</li>`,
		`<a style="color:#0055ff;" href="https://shop.test/sale">The sale</a>`,
		`<p style="margin:0 0 12px;">Clean `,
	} {
		if !strings.Contains(htmlOut, want) {
			t.Errorf("html lacks %q:\n%s", want, htmlOut)
		}
	}
	for _, banned := range []string{"<script", "alert(1)</script>", "onclick", "font-size:90px", "javascript:"} {
		if strings.Contains(htmlOut, banned) {
			t.Errorf("html kept %q:\n%s", banned, htmlOut)
		}
	}
	for _, want := range []string{"Hi <b>Ada</b>, thanks for shopping.", "- Fast shipping", "- The sale (https://shop.test/sale)"} {
		if !strings.Contains(text, want) {
			t.Errorf("plain text lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "<p") || strings.Contains(text, "<li") {
		t.Errorf("plain text kept markup:\n%s", text)
	}
}

func TestAPlainTextBlockRendersAsBefore(t *testing.T) {
	svc := NewService(fakeRepo{}, nil)
	_, htmlOut, text, err := svc.RenderContent(context.Background(), uuid.Nil, "Hi", "", []Block{{Type: "text", Content: "Line one\n<b>not bold</b>"}}, EmailStyles{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(htmlOut, `<p style="margin:0 0 12px;line-height:1.5;text-align:left;color:#333;">Line one<br>&lt;b&gt;not bold&lt;/b&gt;</p>`) {
		t.Errorf("a plain block's html changed:\n%s", htmlOut)
	}
	if !strings.Contains(text, "Line one\n<b>not bold</b>") {
		t.Errorf("a plain block's text changed:\n%s", text)
	}
}

func TestOnlyATextBlockTakesRichText(t *testing.T) {
	for name, b := range map[string]Block{
		"unknown format":      {Type: "text", Format: "markdown", Content: "x"},
		"rich heading":        {Type: "heading", Format: FormatRich, Text: "x"},
		"oversized rich text": {Type: "text", Format: FormatRich, Content: strings.Repeat("a", maxRichContent+1)},
	} {
		if err := validateBlock(b, true); err == nil {
			t.Errorf("%s saved", name)
		}
	}
	if err := validateBlock(Block{Type: "text", Format: FormatRich, Content: "<p>ok</p>"}, true); err != nil {
		t.Errorf("a rich text block refused: %v", err)
	}
}
