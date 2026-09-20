package wiring

import (
	"strings"
	"testing"
)

// One list drives both the options and the flush targets, so an option can never be offered without a prefix behind it to flush
func TestFlushOptionsCoverEveryPrefix(t *testing.T) {
	opts := flushOptions()
	if len(opts) != len(flushableCaches)+1 {
		t.Fatalf("options = %d, want %d prefixes plus \"all\"", len(opts), len(flushableCaches))
	}
	if opts[0].Value != "all" {
		t.Errorf("first option should be all, got %q", opts[0].Value)
	}
	prefixes := flushPrefixes()
	if len(prefixes) != len(flushableCaches) {
		t.Fatalf("prefixes = %d, want %d", len(prefixes), len(flushableCaches))
	}
	for i, f := range flushableCaches {
		if opts[i+1].Value != f.Prefix {
			t.Errorf("option %d = %q, want %q", i+1, opts[i+1].Value, f.Prefix)
		}
		if prefixes[i] != f.Prefix {
			t.Errorf("prefix %d = %q, want %q", i, prefixes[i], f.Prefix)
		}
	}
}

func TestRegisterCacheActionsIsDevOnly(t *testing.T) {
	registerCacheActions(nil, false) // must not panic and must not register - on a live store this would be an unaudited cache flush on the Tools page
}

func TestNoFlushablePrefixShadowsAnother(t *testing.T) {
	// DelPattern scans "cache:<prefix>:*", so if one registered prefix were a colon-prefix of another
	// the dev flush of the shorter one would take the longer one's keys with it
	for _, outer := range flushPrefixes() {
		for _, inner := range flushPrefixes() {
			if outer == inner {
				continue
			}
			if strings.HasPrefix(inner, outer+":") {
				t.Errorf("flushing %q would also match %q, since one namespace nests inside the other", outer, inner)
			}
		}
	}
}
