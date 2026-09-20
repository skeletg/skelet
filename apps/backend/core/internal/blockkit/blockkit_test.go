package blockkit

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseDocument(t *testing.T) {
	if _, ok := ParseDocument(nil); ok {
		t.Fatal("nil body should not parse as a block document")
	}
	// Freeform (no blocks array, no version) is not a block document
	if _, ok := ParseDocument(json.RawMessage(`{"root":{"children":[]}}`)); ok {
		t.Fatal("lexical-style body should not be a block document")
	}
	doc, ok := ParseDocument(json.RawMessage(`{"version":1,"blocks":[{"id":"a","type":"hero"}]}`))
	if !ok || len(doc.Blocks) != 1 || doc.Blocks[0].Type != "hero" {
		t.Fatalf("expected a 1-block document, got ok=%v doc=%+v", ok, doc)
	}
}

func TestValidate(t *testing.T) {
	ok := Document{Version: 1, Blocks: []Block{
		{ID: "1", Type: TypeHero},
		{ID: "2", Type: TypeProductGrid, Props: map[string]any{"source": "newest"}},
		{ID: "3", Type: TypeRef, Props: map[string]any{"slug": "promo"}},
	}}
	if err := Validate(ok, nil); err != nil {
		t.Fatalf("valid doc rejected: %v", err)
	}

	cases := map[string]Document{
		"unknown type": {Blocks: []Block{{ID: "1", Type: "nope"}}},
		"empty type":   {Blocks: []Block{{ID: "1"}}},
		"ref no slug":  {Blocks: []Block{{ID: "1", Type: TypeRef}}},
	}
	for name, doc := range cases {
		if err := Validate(doc, nil); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}

func TestPrepareForFrontSanitizesHTML(t *testing.T) {
	doc := Document{Blocks: []Block{
		{ID: "1", Type: TypeHTML, Props: map[string]any{"html": `<p onclick="x()">hi</p><script>evil()</script>`}},
	}}
	out := PrepareForFront(doc, nil, nil, nil, nil, nil)
	got, _ := out.Blocks[0].Props["html"].(string)
	if got == "" || contains(got, "<script") || contains(got, "onclick") {
		t.Fatalf("html block not sanitized: %q", got)
	}
}

// A CustomRenderer that forgets to sanitize its output must not open a hole -
// prepareBlocks sanitizes the custom-rendered html itself, so the boundary holds
func TestPrepareForFrontSanitizesCustomRenderOutput(t *testing.T) {
	doc := Document{Blocks: []Block{{ID: "1", Type: "my_custom_block"}}}
	custom := func(string, map[string]any) (string, bool) {
		return `<img src=x onerror=alert(1)><p>ok</p>`, true // deliberately unsanitized
	}
	out := PrepareForFront(doc, nil, custom, nil, nil, nil)
	if len(out.Blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(out.Blocks))
	}
	got, _ := out.Blocks[0].Props["html"].(string)
	if contains(got, "onerror") {
		t.Fatalf("custom render output not sanitized: %q", got)
	}
	if !contains(got, "<p>ok</p>") {
		t.Fatalf("safe markup lost: %q", got)
	}
}

func TestPrepareForFrontExpandsRefs(t *testing.T) {
	doc := Document{Blocks: []Block{{ID: "1", Type: TypeRef, Props: map[string]any{"slug": "promo"}}}}
	resolver := func(slug string) ([]Block, bool) {
		if slug == "promo" {
			return []Block{{ID: "h", Type: TypeHero}}, true
		}
		return nil, false
	}
	out := PrepareForFront(doc, resolver, nil, nil, nil, nil)
	if len(out.Blocks) != 1 || out.Blocks[0].Type != TypeHero {
		t.Fatalf("ref not expanded: %+v", out.Blocks)
	}
}

func TestPrepareForFrontResolvesLinkRefs(t *testing.T) {
	doc := Document{Blocks: []Block{{ID: "1", Type: TypeHero, Props: map[string]any{
		"heading":  "Hi",
		"ctaHref":  map[string]any{"type": "reference", "entityType": "cms_page", "entityId": "abc"},
		"cta2Href": map[string]any{"type": "reference", "entityType": "cms_page", "entityId": "gone"},
		"plain":    "/manual",
	}}}}
	links := func(entityType, entityID string) (string, bool) {
		if entityType == "cms_page" && entityID == "abc" {
			return "/about-us", true
		}
		return "", false // "gone" → unresolved
	}
	out := PrepareForFront(doc, nil, nil, links, nil, nil)
	props := out.Blocks[0].Props
	if got, _ := props["ctaHref"].(string); got != "/about-us" {
		t.Fatalf("reference not resolved to URL: %#v", props["ctaHref"])
	}
	if _, present := props["cta2Href"]; present {
		t.Fatalf("unresolved reference should be dropped, got %#v", props["cta2Href"])
	}
	if got, _ := props["plain"].(string); got != "/manual" {
		t.Fatalf("plain string prop should pass through, got %#v", props["plain"])
	}
}

func TestPrepareForFrontResolvesAssetRefs(t *testing.T) {
	doc := Document{Blocks: []Block{{ID: "1", Type: TypeHero, Props: map[string]any{
		"image":   map[string]any{"type": "asset", "assetId": "ok", "alt": "override"},
		"bg":      map[string]any{"type": "asset", "assetId": "gone"}, // deleted → dropped
		"caption": "plain text",
		// repeater items with per-item image refs (gallery/logos pattern)
		"items": []any{
			map[string]any{"url": map[string]any{"type": "asset", "assetId": "ok"}, "label": "a"},
			map[string]any{"label": "no-image"},
		},
	}}}}
	assets := func(id string) (ImageDescriptor, bool) {
		if id == "ok" {
			return ImageDescriptor{Src: "https://cdn/x.jpg", Alt: "default", Width: 800, Height: 600}, true
		}
		return ImageDescriptor{}, false // "gone" → unresolved
	}
	out := PrepareForFront(doc, nil, nil, nil, assets, nil)
	props := out.Blocks[0].Props

	img, ok := props["image"].(map[string]any)
	if !ok {
		t.Fatalf("asset ref not resolved to descriptor: %#v", props["image"])
	}
	if img["src"] != "https://cdn/x.jpg" || img["width"] != 800 || img["height"] != 600 {
		t.Fatalf("descriptor fields wrong: %#v", img)
	}
	if img["alt"] != "override" { // per-usage alt wins over asset default
		t.Fatalf("alt override not applied: %#v", img["alt"])
	}
	if _, present := props["bg"]; present {
		t.Fatalf("unresolved asset ref should be dropped, got %#v", props["bg"])
	}
	if got, _ := props["caption"].(string); got != "plain text" {
		t.Fatalf("plain prop should pass through, got %#v", props["caption"])
	}
	items, _ := props["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items length changed: %#v", items)
	}
	item0, _ := items[0].(map[string]any)
	itemImg, ok := item0["url"].(map[string]any)
	if !ok || itemImg["src"] != "https://cdn/x.jpg" {
		t.Fatalf("repeater-item asset ref not resolved: %#v", item0)
	}
}

func TestFilterServerSide(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	doc := Document{Blocks: []Block{
		{ID: "always", Type: TypeHero, Props: map[string]any{"heading": "Hi"}},
		{ID: "future", Type: TypeHero, Props: map[string]any{"visibility": map[string]any{"startAt": "2099-01-01T00:00:00Z"}}},
		{ID: "expired", Type: TypeHero, Props: map[string]any{"visibility": map[string]any{"endAt": "2000-01-01T00:00:00Z"}}},
		{ID: "vip", Type: TypeHero, Props: map[string]any{"heading": "VIP", "visibility": map[string]any{"segments": []any{"vip"}}}},
		{ID: "device", Type: TypeHero, Props: map[string]any{"visibility": map[string]any{"devices": []any{"mobile"}, "segments": []any{"vip"}}}},
	}}

	// Anonymous viewer (no segments): segment-allowlisted blocks drop
	anon := FilterServerSide(doc, EvalContext{Now: now, Segments: map[string]bool{}})
	ids := blockIDs(anon.Blocks)
	if !sameSet(ids, []string{"always"}) {
		t.Fatalf("anon should see only [always], got %v", ids)
	}

	// VIP member: sees the allowlisted blocks; future/expired still drop
	vip := FilterServerSide(doc, EvalContext{Now: now, Segments: map[string]bool{"vip": true}})
	ids = blockIDs(vip.Blocks)
	if !sameSet(ids, []string{"always", "vip", "device"}) {
		t.Fatalf("vip should see [always vip device], got %v", ids)
	}
	// Audience info stripped; device hint retained
	var deviceBlock *Block
	for i := range vip.Blocks {
		if vip.Blocks[i].ID == "device" {
			deviceBlock = &vip.Blocks[i]
		}
	}
	visM, _ := deviceBlock.Props["visibility"].(map[string]any)
	if _, leaked := visM["segments"]; leaked {
		t.Fatalf("segments should be stripped from shipped props, got %#v", visM)
	}
	if _, ok := visM["devices"]; !ok {
		t.Fatalf("device hint should survive, got %#v", visM)
	}
}

func blockIDs(bs []Block) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.ID
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]bool{}
	for _, x := range a {
		m[x] = true
	}
	for _, x := range b {
		if !m[x] {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestPlainText(t *testing.T) {
	doc := Document{Version: 1, Blocks: []Block{
		{ID: "1", Type: TypeHero, Props: map[string]any{
			"title":    "Summer Sale",
			"subtitle": "Up to 50% off",
			"imageUrl": "https://cdn/x.jpg", // machine prop - must NOT be indexed
			"href":     "/sale",             // machine prop - must NOT be indexed
		}},
		{ID: "2", Type: TypeSection, Blocks: []Block{ // nested child block
			{ID: "3", Type: TypeRichText, Props: map[string]any{"text": "Limited time only"}},
		}},
		{ID: "4", Type: TypeGallery, Props: map[string]any{ // repeater items
			"items": []any{
				map[string]any{"alt": "Sneaker", "image": "https://cdn/s.jpg"},
				map[string]any{"caption": "Boots photo"},
			},
		}},
		{ID: "5", Type: TypeHTML, Props: map[string]any{"html": "<p>Free <b>shipping</b></p>"}},
	}}
	got := PlainText(doc)

	for _, want := range []string{"Summer Sale", "Up to 50% off", "Limited time only", "Sneaker", "Boots photo", "Free", "shipping"} {
		if !contains(got, want) {
			t.Errorf("PlainText missing %q in %q", want, got)
		}
	}
	for _, bad := range []string{"cdn", "/sale", "<p>", "<b>"} {
		if contains(got, bad) {
			t.Errorf("PlainText should not contain machine value %q in %q", bad, got)
		}
	}
}

func TestTranslateRoundTrip(t *testing.T) {
	doc := Document{Version: 1, Blocks: []Block{
		{ID: "1", Type: TypeHero, Props: map[string]any{"heading": "Hello", "imageUrl": "x.jpg", "ctaHref": "/a"}},
		{ID: "2", Type: TypeGallery, Props: map[string]any{"items": []any{
			map[string]any{"alt": "A cat", "image": "c.jpg"},
		}}},
	}}
	strs := TranslatableStrings(doc)
	// "heading" (block 1, sorted before imageUrl; ctaHref not translatable) then
	// the repeater item's "alt"; urls are skipped
	if len(strs) != 2 || strs[0] != "Hello" || strs[1] != "A cat" {
		t.Fatalf("TranslatableStrings = %v, want [Hello, A cat]", strs)
	}
	tr := []string{"X:Hello", "X:A cat"}
	out := ApplyTranslations(doc, tr)
	if out.Blocks[0].Props["heading"] != "X:Hello" {
		t.Errorf("heading not translated: %v", out.Blocks[0].Props["heading"])
	}
	if out.Blocks[0].Props["imageUrl"] != "x.jpg" || out.Blocks[0].Props["ctaHref"] != "/a" {
		t.Errorf("urls should be untouched: %+v", out.Blocks[0].Props)
	}
	if out.Blocks[0].ID != "1" || out.Blocks[1].ID != "2" {
		t.Errorf("block ids must be preserved")
	}
	item := out.Blocks[1].Props["items"].([]any)[0].(map[string]any)
	if item["alt"] != "X:A cat" || item["image"] != "c.jpg" {
		t.Errorf("repeater item alt not translated / image touched: %+v", item)
	}
}
