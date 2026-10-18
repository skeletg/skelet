package cms

import (
	"context"
	"testing"
)

func TestCoerceFieldValue(t *testing.T) {
	cases := []struct {
		name    string
		input   InputType
		value   any
		want    any
		wantErr bool
	}{
		{name: "nil passes through", input: InputText, value: nil, want: nil},
		{name: "text accepts string", input: InputText, value: "hello", want: "hello"},
		{name: "text rejects non-string", input: InputText, value: 5, wantErr: true},
		{name: "number accepts float", input: InputNumber, value: 3.5, want: 3.5},
		{name: "number parses numeric string", input: InputNumber, value: "2.5", want: 2.5},
		{name: "number rejects junk string", input: InputNumber, value: "abc", wantErr: true},
		{name: "number rejects bool", input: InputNumber, value: true, wantErr: true},
		{name: "boolean accepts bool", input: InputBoolean, value: false, want: false},
		{name: "boolean parses yes", input: InputBoolean, value: "yes", want: true},
		{name: "boolean parses off", input: InputBoolean, value: "off", want: false},
		{name: "boolean rejects maybe", input: InputBoolean, value: "maybe", wantErr: true},
		{name: "date accepts string", input: InputDate, value: "2024-01-01", want: "2024-01-01"},
		{name: "date rejects non-string", input: InputDate, value: 1, wantErr: true},
		{name: "asset accepts valid uuid", input: InputAsset, value: "f47ac10b-58cc-4372-a567-0e02b2c3d479", want: "f47ac10b-58cc-4372-a567-0e02b2c3d479"},
		{name: "asset rejects non-uuid", input: InputAsset, value: "not-a-uuid", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := coerceFieldValue(tc.input, tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got value %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("= %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

type fakeDefaultLocale struct{ code string }

func (f fakeDefaultLocale) DefaultLocale(context.Context) (string, error) { return f.code, nil }

// TestFillPageContent exercises the locale fallback selection - the logic class behind the ListPages-untranslated bug:
// requested locale wins, else the global default, else nil
func TestFillPageContent(t *testing.T) {
	ctx := context.Background()
	newPage := func() *Page {
		return &Page{AllContent: []PageContent{
			{Locale: "en", Title: "Hello"},
			{Locale: "de", Title: "Hallo"},
		}}
	}

	t.Run("requested locale wins", func(t *testing.T) {
		ps := &pageService{core: &core{defaultLocale: fakeDefaultLocale{"en"}}}
		p := newPage()
		ps.fillPageContent(ctx, p, "de")
		if p.Content == nil || p.Content.Locale != "de" {
			t.Fatalf("want de content, got %+v", p.Content)
		}
	})

	t.Run("falls back to default locale", func(t *testing.T) {
		ps := &pageService{core: &core{defaultLocale: fakeDefaultLocale{"en"}}}
		p := newPage()
		ps.fillPageContent(ctx, p, "fr") // no fr row
		if p.Content == nil || p.Content.Locale != "en" {
			t.Fatalf("want en fallback, got %+v", p.Content)
		}
	})

	t.Run("no match and no default leaves nil", func(t *testing.T) {
		ps := &pageService{core: &core{defaultLocale: fakeDefaultLocale{""}}}
		p := newPage()
		ps.fillPageContent(ctx, p, "fr")
		if p.Content != nil {
			t.Fatalf("want nil content, got %+v", p.Content)
		}
	})
}
