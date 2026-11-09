package emailtemplate

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// What these pin: each marketing block renders its parts in both the HTML and the text part with merge values escaped,
// the footer's unsubscribe link comes from the send and is left out of one without it, the picture can sit on either
// side, and a block missing what it's for doesn't save
func renderBlocks(t *testing.T, vars map[string]string, blocks ...Block) (string, string) {
	t.Helper()
	_, h, txt, err := NewService(fakeRepo{}, nil).RenderContent(context.Background(), uuid.Nil, "s", "", blocks, EmailStyles{}, vars)
	if err != nil {
		t.Fatal(err)
	}
	return h, txt
}

func mustContain(t *testing.T, where, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("%s lacks %q:\n%s", where, w, got)
		}
	}
}

func TestMarketingBlocksRender(t *testing.T) {
	vars := map[string]string{"customer.firstName": "<Ada>", "unsubscribe.link": "https://shop.test/u/1", "storefront.url": "https://shop.test"}
	h, txt := renderBlocks(t, vars,
		Block{Type: "coupon", Text: "Hi {{customer.firstName}}", Code: "WELCOME10", Content: "Ends Friday", Label: "Shop", URL: "{{storefront.url}}"},
		Block{Type: "store_footer", Content: "Skelet Ltd\n1 Main St", Label: "Stop these"},
		Block{Type: "menu", Links: []MenuLink{{Label: "Sale", URL: "{{storefront.url}}/sale"}, {Label: "New", URL: "https://shop.test/new"}}},
		Block{Type: "image_text", Image: "https://cdn.test/a.png", Alt: "Hat", Text: "The hat", Content: "Warm", Label: "Buy", URL: "https://shop.test/hat", Side: "right"},
	)
	mustContain(t, "html", h,
		`Hi &lt;Ada&gt;`, `>WELCOME10</div>`, `Ends Friday`, `href="https://shop.test"`,
		`Skelet Ltd<br>1 Main St`, `<a href="https://shop.test/u/1" style="color:#888;text-decoration:underline;">Stop these</a>`,
		`href="https://shop.test/sale"`, `>Sale</a><span style="color:#ccc;padding:0 10px;">|</span><a`,
		`class="skl-stack"`, `<img src="https://cdn.test/a.png" alt="Hat"`, `.skl-stack{display:block!important`,
	)
	if strings.Index(h, "The hat") > strings.Index(h, "cdn.test/a.png") {
		t.Error("a picture on the right comes after its words")
	}
	mustContain(t, "text", txt, "Hi <Ada>\nCode: WELCOME10\nEnds Friday\nShop: https://shop.test\n", "Stop these: https://shop.test/u/1", "Sale: https://shop.test/sale\nNew: https://shop.test/new", "The hat\nWarm\nBuy: https://shop.test/hat")

	h, txt = renderBlocks(t, map[string]string{}, Block{Type: "store_footer", Content: "Skelet Ltd"})
	if strings.Contains(h, "Unsubscribe") || strings.Contains(txt, "Unsubscribe") {
		t.Error("a send with no unsubscribe link still printed one")
	}
	h, _ = renderBlocks(t, nil, Block{Type: "image_text", Image: "https://cdn.test/a.png", Text: "Left"})
	if strings.Index(h, "Left") < strings.Index(h, "cdn.test/a.png") {
		t.Error("a picture on the left comes after its words")
	}
	h, _ = renderBlocks(t, nil, Block{Type: "image_text", Text: "Words only"})
	if strings.Contains(h, `class="skl-stack"`) || !strings.Contains(h, "Words only") {
		t.Errorf("no picture should leave the words full width, without the two-cell table:\n%s", h)
	}
}

func TestMarketingBlocksNeedWhatTheyAreFor(t *testing.T) {
	for _, b := range []Block{
		{Type: "coupon"},
		{Type: "coupon", Code: "X", URL: "javascript:alert(1)"},
		{Type: "menu"},
		{Type: "menu", Links: []MenuLink{{Label: "", URL: "https://a.test"}}},
		{Type: "menu", Links: []MenuLink{{Label: "Bad", URL: "javascript:x"}}},
		{Type: "image_text", Image: "https://a.test/x.png", Side: "top"},
		{Type: "image_text", Image: "javascript:x"},
	} {
		if err := validateBlock(b, true); err == nil {
			t.Errorf("%+v saved", b)
		}
	}
	for _, b := range []Block{
		{Type: "coupon", Code: "{{coupon.code}}"},
		{Type: "store_footer"},
		{Type: "menu", Links: []MenuLink{{Label: "Sale", URL: "{{storefront.url}}/sale"}}},
		{Type: "image_text", Image: "{{store.logoUrl}}", Side: "left"},
		{Type: "image_text", Text: "No picture yet"},
	} {
		if err := validateBlock(b, true); err != nil {
			t.Errorf("%+v: %v", b, err)
		}
	}
}

// A built-in type and a reusable block share one set of names - a new type that takes a seeded block's slug stops the
// starter content seeding at all
func TestNoSeededBlockTakesABuiltinName(t *testing.T) {
	for _, sn := range standardBlocks() {
		if isBuiltinType(sn.Slug) {
			t.Errorf("seeded block %q has a built-in block type's name", sn.Slug)
		}
	}
}
