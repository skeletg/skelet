package linkgraph

import (
	"slices"
	"testing"
)

func TestCanonicalMustPointHereOnThisSite(t *testing.T) {
	for _, tc := range []struct {
		canonical, path string
		want            bool
	}{
		{"https://shop.test/shoes", "/shoes", true},
		{"https://SHOP.test/shoes/", "/shoes", true},
		{"/shoes", "/shoes", true},
		{"https://shop.test", "/", true},
		{"https://shop.test/boots", "/shoes", false},
		// The classic launch mistake - every canonical still names the staging host
		{"https://staging.shop.test/shoes", "/shoes", false},
	} {
		if got := canonicalPointsHere(tc.canonical, tc.path, "shop.test"); got != tc.want {
			t.Errorf("canonical %q on %q = %v, want %v", tc.canonical, tc.path, got, tc.want)
		}
	}
}

func TestSkippedHeadingLevel(t *testing.T) {
	if !skippedHeadingLevel([]Heading{{Level: 1}, {Level: 3}}) {
		t.Error("h1 then h3 not flagged")
	}
	if skippedHeadingLevel([]Heading{{Level: 1}, {Level: 2}, {Level: 3}, {Level: 2}}) {
		t.Error("a well-formed outline flagged")
	}
}

func TestFlagIssues(t *testing.T) {
	good := AuditPage{Path: "/shoes", StatusCode: 200, Title: "Running shoes for every road", TitleLength: 28,
		MetaDesc: "x", MetaLength: 80, H1Count: 1, Headings: []Heading{{Level: 1}}, Canonical: "/shoes",
		InLinks: 3, Depth: 1, WordCount: 400, InternalLinks: 10}
	if got := flagIssues(&good, map[string]int{}, map[string]int{}, "shop.test"); len(got) != 0 {
		t.Errorf("a healthy page flagged %v", got)
	}

	broken := AuditPage{Path: "/gone", StatusCode: 404}
	if got := flagIssues(&broken, nil, nil, ""); !slices.Equal(got, []Issue{IssueBroken}) {
		t.Errorf("a broken page = %v, want BROKEN alone", got)
	}

	bad := good
	bad.Title, bad.TitleLength = "Shoes", 5
	bad.MetaLength = 0
	bad.Canonical = "https://staging.test/shoes"
	bad.InLinks, bad.Depth, bad.WordCount = 0, 5, 10
	got := flagIssues(&bad, map[string]int{"shoes": 2}, map[string]int{}, "shop.test")
	for _, want := range []Issue{IssueTitleShort, IssueTitleDuplicate, IssueMetaMissing, IssueCanonicalOther, IssueOrphan, IssueDeep, IssueThin} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
}
