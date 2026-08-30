package search

import "testing"

func groupSlugs(fgs []FacetGroup) []string {
	out := make([]string, len(fgs))
	for i, fg := range fgs {
		out[i] = fg.Slug
	}
	return out
}

func TestApplyFacetOverrides(t *testing.T) {
	base := func() []FacetGroup {
		return []FacetGroup{
			{Slug: "brand"}, {Slug: "color"}, {Slug: "size"}, {Slug: "material"},
		}
	}

	t.Run("no overrides is a no-op", func(t *testing.T) {
		got := ApplyFacetOverrides(base(), nil, nil)
		want := []string{"brand", "color", "size", "material"}
		for i, s := range groupSlugs(got) {
			if s != want[i] {
				t.Fatalf("order changed with no overrides: %v", groupSlugs(got))
			}
		}
	})

	t.Run("pinned facets lead in pin order, rest keep incoming order", func(t *testing.T) {
		got := ApplyFacetOverrides(base(), map[string]int32{"size": 0, "brand": 1}, nil)
		want := []string{"size", "brand", "color", "material"}
		for i, s := range groupSlugs(got) {
			if s != want[i] {
				t.Fatalf("got %v want %v", groupSlugs(got), want)
			}
		}
	})

	t.Run("hidden facets drop even when also pinned", func(t *testing.T) {
		got := ApplyFacetOverrides(
			base(),
			map[string]int32{"color": 0},
			map[string]bool{"color": true, "material": true},
		)
		want := []string{"brand", "size"}
		if len(got) != len(want) {
			t.Fatalf("got %v want %v", groupSlugs(got), want)
		}
		for i, s := range groupSlugs(got) {
			if s != want[i] {
				t.Fatalf("got %v want %v", groupSlugs(got), want)
			}
		}
	})

	t.Run("override for an absent facet is harmless", func(t *testing.T) {
		got := ApplyFacetOverrides(base(), map[string]int32{"ghost": 0}, map[string]bool{"phantom": true})
		if len(got) != 4 {
			t.Fatalf("dropped facets it shouldn't have: %v", groupSlugs(got))
		}
	})
}
