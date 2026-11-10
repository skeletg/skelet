package shoptools

import (
	"testing"

	"github.com/google/uuid"
)

func TestARecapReadsBackWhatItWrote(t *testing.T) {
	dress, skirt, coat := uuid.New(), uuid.New(), uuid.New()
	c := &conversation{shown: []uuid.UUID{dress, skirt}, detail: &Detail{ProductID: coat}, checkout: true,
		links: []Link{{Label: `The "linen" edit`, Path: "/shop/linen"}}, navigate: &Link{Path: "/search?q=linen"},
		returns: []string{"Return R1a2b3c4d summarised"}}
	c.remember(dress, "Smocked Midi Dress. In white")
	c.remember(coat, "Rain Coat")
	got := ReadRecap(c.recap())
	want := []Shown{
		{Kind: ShownPanel, Name: "Rain Coat", ProductID: coat},
		{Kind: ShownCard, Name: "Smocked Midi Dress. In white", ProductID: dress},
		{Kind: ShownCard, ProductID: skirt},
		{Kind: ShownButton, Label: `The "linen" edit`, Path: "/shop/linen"},
		{Kind: ShownMoved, Path: "/search?q=linen"},
		{Kind: ShownCart},
	}
	if len(got) != len(want) {
		t.Fatalf("read %+v from %q", got, c.recap())
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if s := ReadRecap("something the model wrote"); len(s) != 1 || s[0].Kind != ShownText {
		t.Errorf("an unreadable recap = %+v - want it kept as text", s)
	}
	if ReadRecap("  ") != nil {
		t.Error("an empty recap read as something shown")
	}
	listed := []OverviewProduct{{ID: dress, Name: "Smocked Midi Dress"}, {ID: skirt, Name: "Wrap Skirt"}}
	o := ReadRecap(OverviewRecap(Overview{Text: "[[2]] is airy, and so is [[2]].", Products: listed}))
	if len(o) != 1 || o[0].Kind != ShownCard || o[0].ProductID != skirt {
		t.Errorf("an overview's recap = %+v - want the one product it names, once", o)
	}
	if all := ReadRecap(OverviewRecap(Overview{Text: "Both suit.", Products: listed})); len(all) != 2 {
		t.Errorf("an overview naming none = %+v - want every product it lists", all)
	}
}
