package cms

import (
	"testing"

	"github.com/google/uuid"
)

func TestJoinRoot(t *testing.T) { // the page's public path is root + base, and a stray slash either doubles a separator or swallows a segment
	for name, tc := range map[string]struct{ root, base, want string }{
		"both":               {"help", "getting-started", "help/getting-started"},
		"no root":            {"", "about", "about"},
		"blank root":         {"   ", "about", "about"},
		"slashed root":       {"/help/", "about", "help/about"},
		"nested root":        {"help/articles", "faq", "help/articles/faq"},
		"no base":            {"help", "", "help"},
		"neither":            {"", "", ""},
		"nested base":        {"help", "a/b/c", "help/a/b/c"},
		"root of only slash": {"/", "about", "about"},
	} {
		if got := joinRoot(tc.root, tc.base); got != tc.want {
			t.Errorf("%s: joinRoot(%q, %q) = %q, want %q", name, tc.root, tc.base, got, tc.want)
		}
	}
}

func TestJoinRootNeverProducesADoubleOrLeadingSlash(t *testing.T) { // a doubled separator is a 404 and a leading one changes what the router matches
	for _, root := range []string{"", "/", "help", "/help", "help/", "/help/", "  /help/  "} {
		for _, base := range []string{"", "about", "a/b"} {
			got := joinRoot(root, base)
			if len(got) > 0 && got[0] == '/' {
				t.Errorf("joinRoot(%q, %q) = %q, want no leading slash", root, base, got)
			}
			for i := 1; i < len(got); i++ {
				if got[i] == '/' && got[i-1] == '/' {
					t.Errorf("joinRoot(%q, %q) = %q, which doubles a separator", root, base, got)
					break
				}
			}
		}
	}
}

func TestChannelKey(t *testing.T) { // the global scope has to key distinctly from every channel, since the reconcile diffs old against wanted by this key
	if got := channelKey(nil); got != "" {
		t.Errorf("channelKey(nil) = %q, want the empty global key", got)
	}
	a, b := uuid.New(), uuid.New()
	if channelKey(&a) == channelKey(&b) {
		t.Error("two channels share a key, so one would overwrite the other in the reconcile map")
	}
	if channelKey(&a) == "" {
		t.Error("a channel keys as global, so its canonical would be treated as the global one")
	}
}

func pathsOf(sps []scopedPath) []string {
	out := make([]string, 0, len(sps))
	for _, sp := range sps {
		out = append(out, sp.path)
	}
	return out
}

func TestDesiredPageURLsAlwaysLeadsWithTheGlobalPath(t *testing.T) { // syncPageURLs takes want[0] as the global canonical, so the order is load-bearing rather than cosmetic
	ch := uuid.New()
	pt := PageType{RootSlug: "help", ChannelRoots: []PageTypeChannelRoot{{ChannelID: ch, RootSlug: "support"}}}

	got := desiredPageURLs("faq", pt)
	if len(got) != 2 {
		t.Fatalf("got %v, want the global path plus one channel override", pathsOf(got))
	}
	if got[0].channelID != nil {
		t.Error("the first entry is channel-scoped, but syncPageURLs reads it as the global canonical")
	}
	if got[0].path != "help/faq" {
		t.Errorf("global path = %q, want help/faq", got[0].path)
	}
	if got[1].channelID == nil || *got[1].channelID != ch {
		t.Errorf("the override is scoped to %v, want channel %v", got[1].channelID, ch)
	}
	if got[1].path != "support/faq" {
		t.Errorf("override path = %q, want support/faq", got[1].path)
	}
}

func TestDesiredPageURLsSkipsAnOverrideMatchingGlobal(t *testing.T) { // a per-channel canonical is only persisted where the path DIFFERS - an identical one is a redundant row the global canonical already serves
	same, differs := uuid.New(), uuid.New()
	pt := PageType{RootSlug: "help", ChannelRoots: []PageTypeChannelRoot{
		{ChannelID: same, RootSlug: "help"},    // identical to global
		{ChannelID: differs, RootSlug: "docs"}, // genuinely different
	}}

	got := desiredPageURLs("faq", pt)
	if len(got) != 2 {
		t.Fatalf("got %v, want only the global path and the differing override", pathsOf(got))
	}
	for _, sp := range got {
		if sp.channelID != nil && *sp.channelID == same {
			t.Errorf("an override equal to the global path was kept as %q, want it skipped", sp.path)
		}
	}
}

func TestDesiredPageURLsTreatsSlashVariantsAsTheSameRoot(t *testing.T) { // joinRoot trims, so "/help/" and "help" produce one path - keeping both would write a duplicate canonical
	ch := uuid.New()
	pt := PageType{RootSlug: "help", ChannelRoots: []PageTypeChannelRoot{{ChannelID: ch, RootSlug: "/help/"}}}

	got := desiredPageURLs("faq", pt)
	if len(got) != 1 {
		t.Errorf("got %v, want the slash variant recognized as the global root", pathsOf(got))
	}
}

func TestDesiredPageURLsWithNoRootAtAll(t *testing.T) { // a bare page type serves its pages at the top level, so the path is just the base
	got := desiredPageURLs("about", PageType{})
	if len(got) != 1 || got[0].path != "about" || got[0].channelID != nil {
		t.Errorf("got %v, want a single global \"about\"", pathsOf(got))
	}
}

func TestDesiredPageURLsCoversEveryDifferingChannel(t *testing.T) { // a dropped channel means that store's pages resolve through the global path and silently lose their prefix
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	pt := PageType{RootSlug: "help", ChannelRoots: []PageTypeChannelRoot{
		{ChannelID: a, RootSlug: "aide"},
		{ChannelID: b, RootSlug: "hilfe"},
		{ChannelID: c, RootSlug: ""}, // bare root differs from "help" too
	}}

	got := desiredPageURLs("faq", pt)
	if len(got) != 4 {
		t.Fatalf("got %v, want the global plus all three differing channels", pathsOf(got))
	}
	byChannel := map[uuid.UUID]string{}
	for _, sp := range got {
		if sp.channelID != nil {
			byChannel[*sp.channelID] = sp.path
		}
	}
	for ch, want := range map[uuid.UUID]string{a: "aide/faq", b: "hilfe/faq", c: "faq"} {
		if byChannel[ch] != want {
			t.Errorf("channel %v path = %q, want %q", ch, byChannel[ch], want)
		}
	}
}

func TestDesiredPageURLsAgreesWithEffectiveRoot(t *testing.T) { // EffectiveRoot is what read paths resolve through, so a disagreement means the stored canonical and the resolver differ
	a, b := uuid.New(), uuid.New()
	pt := PageType{RootSlug: "help", ChannelRoots: []PageTypeChannelRoot{
		{ChannelID: a, RootSlug: "aide"},
		{ChannelID: b, RootSlug: "help"},
	}}
	const base = "faq"

	for _, sp := range desiredPageURLs(base, pt) {
		want := joinRoot(pt.EffectiveRoot(sp.channelID), base)
		if sp.path != want {
			t.Errorf("desired path for channel %v = %q, but EffectiveRoot resolves %q", sp.channelID, sp.path, want)
		}
	}
	// and the channel whose override equals global still resolves through the global canonical
	if got := joinRoot(pt.EffectiveRoot(&b), base); got != "help/faq" {
		t.Errorf("the matching channel resolves %q, want the global help/faq", got)
	}
}

func TestEffectiveRoot(t *testing.T) { // a channel with no override has to fall back to the type's global root rather than to bare
	a, unknown := uuid.New(), uuid.New()
	pt := PageType{RootSlug: "help", ChannelRoots: []PageTypeChannelRoot{{ChannelID: a, RootSlug: "aide"}}}

	if got := pt.EffectiveRoot(nil); got != "help" {
		t.Errorf("EffectiveRoot(nil) = %q, want the global root", got)
	}
	if got := pt.EffectiveRoot(&a); got != "aide" {
		t.Errorf("EffectiveRoot(override) = %q, want aide", got)
	}
	if got := pt.EffectiveRoot(&unknown); got != "help" {
		t.Errorf("EffectiveRoot(no override) = %q, want the global root", got)
	}
	// an override to bare is a real choice, not an absent one
	bare := PageType{RootSlug: "help", ChannelRoots: []PageTypeChannelRoot{{ChannelID: a, RootSlug: ""}}}
	if got := bare.EffectiveRoot(&a); got != "" {
		t.Errorf("an override to the bare root = %q, want it honored as empty", got)
	}
}
