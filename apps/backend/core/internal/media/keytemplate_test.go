package media

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

var (
	keyID = uuid.MustParse("018f0000-0000-7000-8000-0000deadbeef")
	keyAt = time.Date(2026, 3, 9, 23, 30, 0, 0, time.FixedZone("late", 2*3600)) // already the 9th in UTC
)

func TestAKeyIsTheTemplateWithItsSlotsFilled(t *testing.T) {
	v := KeyValues{ID: keyID, Name: "Summer Sale (final).JPEG", Ext: ".jpg", At: keyAt, More: map[string]string{"sku": "AB/12 x", "type": "gallery"}}
	for _, tc := range []struct {
		template, want string
		folders        bool
	}{
		{"{id}-{name}{ext}", keyID.String() + "-summer-sale-final.jpg", false},
		{"{name}_{short_id}{ext}", "summer-sale-final_deadbeef.jpg", false},
		{"  {short_id}{ext}  ", "deadbeef.jpg", false},
		{"products/{sku}/{type}/{yyyy}/{mm}/{dd}/{name}-{short_id}{ext}", "products/ab-12-x/gallery/2026/03/09/summer-sale-final-deadbeef.jpg", true},
		{"p/{sku}.{id}{ext}", "p/ab-12-x." + keyID.String() + ".jpg", true},
		{"v1.2/{name}.{short_id}{ext}", "v1.2/summer-sale-final.deadbeef.jpg", true},
	} {
		tpl, err := ParseKeyTemplate(tc.template, tc.folders, "sku", "type")
		if err != nil {
			t.Errorf("%q refused: %v", tc.template, err)
			continue
		}
		if got := tpl.Key(v); got != tc.want {
			t.Errorf("%q gave %q, want %q", tc.template, got, tc.want)
		}
	}
}

// Whatever a slot is handed, it can't add a folder, climb out of one or leave a segment empty
func TestASlotNeverChangesTheShapeOfTheKey(t *testing.T) {
	tpl, err := ParseKeyTemplate("products/{sku}/{name}-{id}{ext}", true, "sku")
	if err != nil {
		t.Fatal(err)
	}
	for _, hostile := range []string{"../../data/exports", "..", ".", "", "   ", "a/b", `a\b`, "é", "%2e%2e%2f", "a..b", "x...", strings.Repeat("long", 400)} {
		key := tpl.Key(KeyValues{ID: keyID, Name: hostile + ".png", Ext: ".png", More: map[string]string{"sku": hostile}})
		segs := strings.Split(key, "/")
		if len(segs) != 3 || segs[0] != "products" {
			t.Errorf("%q reshaped the key: %s", hostile, key)
		}
		if strings.Contains(key, "..") || len(key) > 2*keyPartMax+100 {
			t.Errorf("%q made a key that can't be served: %s", hostile, key)
		}
		for _, seg := range segs {
			if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, `\%`) {
				t.Errorf("%q left the segment %q in %s", hostile, seg, key)
			}
		}
	}
}

func TestATemplateThatCouldNotMakeASafeKeyIsRefused(t *testing.T) {
	for _, tc := range []struct {
		template, says string
		folders        bool
	}{
		{"", "empty", false},
		{"   ", "empty", false},
		{"{id}-{name}", "end with {ext}", false},
		{"{id}{ext}.bak", "end with {ext}", false},
		{"{name}{ext}", "{id} or {short_id}", false},
		{"logo{ext}", "{id} or {short_id}", false},
		{"{id}-{sku}{ext}", "isn't a placeholder here", false},
		{"{id}-{Name}{ext}", "isn't a placeholder here", false},
		{"{id}-{}{ext}", "isn't a placeholder here", false},
		{"{id}-{ name }{ext}", "isn't a placeholder here", false},
		{"{id}-{name{ext}", "never closed", false},
		{"{id}-{na{me}}{ext}", "never closed", false},
		{"{id}}-{name}{ext}", "closes nothing", false},
		{"{yyyy}/{id}{ext}", "can't be part", false},
		{"{id} {name}{ext}", "can't be part", false},
		{"{id}?v=1{ext}", "can't be part", false},
		{"{id}%2f{ext}", "can't be part", false},
		{`a\{id}{ext}`, "can't be part", true},
		{"/products/{id}{ext}", "needs a name", true},
		{"products//{id}{ext}", "needs a name", true},
		{"products/{id}/{ext}", "needs a name", true},
		{"products/../{id}{ext}", "needs a name", true},
		{"./{id}{ext}", "needs a name", true},
		{"a..b-{id}{ext}", "two dots", false},
		{"{id}.{ext}", "two dots", false},
		{"products/{id}..{name}{ext}", "two dots", true},
		{strings.Repeat("a", keyTemplateMax) + "{id}{ext}", "longer than", true},
	} {
		_, err := ParseKeyTemplate(tc.template, tc.folders)
		if !errors.Is(err, ErrKeyTemplate) || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%q: got %v, want a refusal saying %q", tc.template, err, tc.says)
		}
	}
}

func TestEveryShippedLayoutParsesAndIsCheckedOnSave(t *testing.T) {
	for _, k := range []KeySetting{LibraryFileKey, ProductImageKey, CategoryCoverKey, CollectionCoverKey} {
		if _, err := k.Parse(k.Default); err != nil {
			t.Errorf("%s ships a default that doesn't parse: %v", k.Path, err)
		}
		def, ok := config.Find(k.Path)
		if !ok || def.Check == nil || def.Default != k.Default {
			t.Errorf("%s isn't registered with its default and its check", k.Path)
			continue
		}
		if err := def.Check("{name}{ext}"); err == nil {
			t.Errorf("%s would save a template that reuses addresses", k.Path)
		}
		if err := def.Check(""); err != nil {
			t.Errorf("%s refuses the empty value that means the default: %v", k.Path, err)
		}
		for _, slot := range k.Slots {
			if !strings.Contains(def.Description, "{"+slot+"}") {
				t.Errorf("%s doesn't tell the admin about {%s}", k.Path, slot)
			}
		}
	}
	if _, err := LibraryFileKey.Parse("{yyyy}/{id}{ext}"); err == nil {
		t.Error("a library file name may hold a folder, which would file it away from the folder it was put in")
	}
}

type savedKeys map[string]string

func (s savedKeys) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	v, ok := s[path]
	if !ok {
		return "", errors.New("settings unavailable")
	}
	return v, nil
}

func TestTheDefaultStandsInWheneverTheSavedLayoutCantBeUsed(t *testing.T) {
	v := KeyValues{ID: keyID, Name: "a.png", Ext: ".png", At: keyAt, More: map[string]string{"category_id": "c1", "slug": "Shoes"}}
	byDefault := "categories/c1/cover/" + keyID.String() + ".png"
	for name, tc := range map[string]struct {
		cfg  KeyConfig
		want string
	}{
		"no settings":      {nil, byDefault},
		"settings failing": {savedKeys{}, byDefault},
		"nothing saved":    {savedKeys{CategoryCoverKey.Path: "  "}, byDefault},
		"saved and broken": {savedKeys{CategoryCoverKey.Path: "covers/{slug}{ext}"}, byDefault},
		"saved":            {savedKeys{CategoryCoverKey.Path: "covers/{slug}-{short_id}{ext}"}, "covers/shoes-deadbeef.png"},
	} {
		if got := CategoryCoverKey.Template(context.Background(), tc.cfg).Key(v); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}
