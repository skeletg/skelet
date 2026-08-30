package cache

import "testing"

// One list drives both the options and the flush targets, so an option can never be offered without a prefix behind it to flush
func TestFlushOptionsCoverEveryPrefix(t *testing.T) {
	opts := flushOptions()
	if len(opts) != len(flushable)+1 {
		t.Fatalf("options = %d, want %d prefixes plus \"all\"", len(opts), len(flushable))
	}
	if opts[0].Value != "all" {
		t.Errorf("first option should be all, got %q", opts[0].Value)
	}
	prefixes := flushPrefixes()
	if len(prefixes) != len(flushable) {
		t.Fatalf("prefixes = %d, want %d", len(prefixes), len(flushable))
	}
	for i, f := range flushable {
		if opts[i+1].Value != f.Prefix {
			t.Errorf("option %d = %q, want %q", i+1, opts[i+1].Value, f.Prefix)
		}
		if prefixes[i] != f.Prefix {
			t.Errorf("prefix %d = %q, want %q", i, prefixes[i], f.Prefix)
		}
	}
}

func TestRegisterActionsIsDevOnly(t *testing.T) {
	RegisterActions(nil, false) // must not panic and must not register - on a live store this would be an unaudited cache flush on the Tools page
}
