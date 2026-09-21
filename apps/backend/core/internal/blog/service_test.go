package blog

import (
	"context"
	"testing"
)

type fakeDefaultLocale struct{ code string }

func (f fakeDefaultLocale) DefaultLocale(context.Context) (string, error) { return f.code, nil }

// TestFillPostContent exercises the locale fallback selection shared across the
// blog read paths: requested locale wins, else the global default, else nil
func TestFillPostContent(t *testing.T) {
	ctx := context.Background()
	newPost := func() *Post {
		return &Post{AllContent: []PostContent{
			{Locale: "en", Title: "Hello"},
			{Locale: "de", Title: "Hallo"},
		}}
	}

	t.Run("requested locale wins", func(t *testing.T) {
		c := &core{defaultLocale: fakeDefaultLocale{"en"}}
		p := newPost()
		c.fillPostContent(ctx, p, "de")
		if p.Content == nil || p.Content.Locale != "de" {
			t.Fatalf("want de content, got %+v", p.Content)
		}
	})

	t.Run("falls back to default locale", func(t *testing.T) {
		c := &core{defaultLocale: fakeDefaultLocale{"en"}}
		p := newPost()
		c.fillPostContent(ctx, p, "fr") // no fr row
		if p.Content == nil || p.Content.Locale != "en" {
			t.Fatalf("want en fallback, got %+v", p.Content)
		}
	})

	t.Run("no match and no default leaves nil", func(t *testing.T) {
		c := &core{defaultLocale: fakeDefaultLocale{""}}
		p := newPost()
		c.fillPostContent(ctx, p, "fr")
		if p.Content != nil {
			t.Fatalf("want nil content, got %+v", p.Content)
		}
	})

	t.Run("requested locale present but no fallback needed", func(t *testing.T) {
		c := &core{defaultLocale: fakeDefaultLocale{"en"}}
		p := newPost()
		c.fillPostContent(ctx, p, "en")
		if p.Content == nil || p.Content.Locale != "en" {
			t.Fatalf("want en content, got %+v", p.Content)
		}
	})
}
