package searchmerch

import (
	"strings"
	"testing"
)

// Normalize is the key redirects and pins are stored and looked up under, so any drift between the
// write path and the read path makes a configured redirect silently never fire
func TestNormalizeIsTheCanonicalQueryKey(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"tires", "tires"},
		{"TIRES", "tires"},
		{"  tires  ", "tires"},
		{"Winter Tires", "winter tires"},
		{"winter   tires", "winter tires"},
		{"\twinter\ntires ", "winter tires"},
		{"", ""},
		{"   ", ""},
		{"205/55R16", "205/55r16"},
	} {
		if got := Normalize(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Every spelling a shopper might type has to land on one key, or half of them miss the redirect
func TestNormalizeCollapsesEquivalentSpellings(t *testing.T) {
	variants := []string{"Winter Tires", "winter tires", "  WINTER   TIRES  ", "winter\ttires"}
	want := Normalize(variants[0])
	for _, v := range variants[1:] {
		if got := Normalize(v); got != want {
			t.Errorf("Normalize(%q) = %q, want it to match %q from %q", v, got, want, variants[0])
		}
	}
}

// Normalizing an already-normalized key must not change it, or a re-save would move the row
func TestNormalizeIsIdempotent(t *testing.T) {
	for _, in := range []string{"winter tires", "", "205/55r16", "a b c"} {
		once := Normalize(in)
		if twice := Normalize(once); twice != once {
			t.Errorf("Normalize(Normalize(%q)) = %q, want %q", in, twice, once)
		}
	}
}

// Admins type synonym groups by hand into one text field, so the parser absorbs the usual sloppiness
func TestSplitTerms(t *testing.T) {
	for name, tc := range map[string]struct {
		in   string
		want []string
	}{
		"plain":            {"tyre,tire,tires", []string{"tyre", "tire", "tires"}},
		"padded":           {"tyre, tire , tires", []string{"tyre", "tire", "tires"}},
		"mixed case":       {"Tyre, TIRE", []string{"tyre", "tire"}},
		"empty segments":   {"tyre,,tire,", []string{"tyre", "tire"}},
		"duplicates":       {"tyre, tire, tyre, TYRE", []string{"tyre", "tire"}},
		"single term":      {"tyre", []string{"tyre"}},
		"multi-word terms": {"winter tyre, snow tire", []string{"winter tyre", "snow tire"}},
		"nothing":          {"", nil},
		"only separators":  {" , , ", nil},
	} {
		got := splitTerms(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("%s: splitTerms(%q) = %v, want %v", name, tc.in, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("%s: splitTerms(%q) = %v, want %v", name, tc.in, got, tc.want)
				break
			}
		}
	}
}

// Order is preserved so the admin sees the group back the way they typed it
func TestSplitTermsKeepsTheAdminsOrder(t *testing.T) {
	got := splitTerms("zebra, alpha, middle")
	want := []string{"zebra", "alpha", "middle"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitTerms = %v, want %v", got, want)
		}
	}
}

// A synonym group only matches if its terms are comparable to the query's terms, and ExpandQuery
// lowercases with strings.Fields - so a term carrying stray case or padding could never hit
func TestSplitTermsOutputIsComparableToQueryTerms(t *testing.T) {
	for _, term := range splitTerms(" Tyre , TIRE ,tires ") {
		if term != strings.ToLower(term) {
			t.Errorf("term %q is not lowercased, so a lowercased query term can never match it", term)
		}
		if term != strings.TrimSpace(term) {
			t.Errorf("term %q carries whitespace, so it can never match a query field", term)
		}
	}
}
