package behavioralsegment

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// Parse is what separates a behavioral segment from a hand-managed one, so everything the scheduler
// and the mutations touch hangs off it answering correctly
func TestParseRejectsWhatIsNotBehavioral(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":          "",
		"blank":          "   ",
		"not json":       "SELECT id FROM customers",
		"json but bare":  `{"channelId":"c"}`,
		"empty criteria": `{"channelId":"c","conditions":[],"trait":{},"affinity":{}}`,
	} {
		if _, ok := Parse(raw); ok {
			t.Errorf("%s parsed as behavioral, but it is not", name)
		}
	}
}

// A tree-only definition carries none of the flat fields, so a check that looks at those first reads
// every tree segment as non-behavioral and silently stops refreshing it
func TestParseAcceptsATreeWithNoFlatFields(t *testing.T) {
	raw := `{"channelId":"c","root":{"kind":"group","op":"or","children":[{"kind":"event","event":"page_view"}]}}`
	def, ok := Parse(raw)
	if !ok {
		t.Fatal("a tree-only definition has to parse as behavioral")
	}
	if def.Root == nil || def.Root.Op != OpOr || len(def.Root.Children) != 1 {
		t.Fatalf("tree did not survive the round trip: %+v", def.Root)
	}
	if got := def.Tree(); got != def.Root {
		t.Error("Tree has to hand back the stored root, not a derived one")
	}
}

func TestParseAcceptsEachFlatCriterionAlone(t *testing.T) {
	for name, raw := range map[string]string{
		"condition": `{"channelId":"c","conditions":[{"event":"add_to_cart"}]}`,
		"trait":     `{"channelId":"c","trait":{"lifecycle":["active"]}}`,
		"affinity":  `{"channelId":"c","affinity":{"attr":"brand","value":"acme"}}`,
	} {
		if _, ok := Parse(raw); !ok {
			t.Errorf("a %s-only definition has to parse as behavioral", name)
		}
	}
}

// Flat definitions evaluate through the same tree path as authored ones, so the derived root has to
// carry every criterion, AND'd
func TestTreeDerivesAnAndGroupFromTheFlatFields(t *testing.T) {
	def := Definition{
		ChannelID:  uuid.New().String(),
		Conditions: []Condition{{Event: "page_view"}, {Event: "checkout", Negate: true}},
		Trait:      &Trait{Lifecycle: []string{"active"}},
		Affinity:   &Affinity{Attr: "brand", Value: "acme"},
	}
	root := def.Tree()
	if root == nil {
		t.Fatal("a definition with criteria has to derive a root")
	}
	if root.Kind != KindGroup || root.Op != OpAnd {
		t.Fatalf("derived root is %s/%s, want an AND group", root.Kind, root.Op)
	}
	if len(root.Children) != 4 {
		t.Fatalf("derived %d children, want one per criterion: %+v", len(root.Children), root.Children)
	}
	kinds := map[string]int{}
	for _, c := range root.Children {
		kinds[c.Kind]++
	}
	if kinds[KindEvent] != 2 || kinds[KindTrait] != 1 || kinds[KindAffinity] != 1 {
		t.Errorf("derived kinds %v, want 2 events + 1 trait + 1 affinity", kinds)
	}
	if !root.Children[1].Negate {
		t.Error("a negated condition has to stay negated in the derived tree")
	}
}

// An inactive trait or affinity is not a criterion, and a tree built from one would match everyone
func TestTreeIgnoresInactiveTraitAndAffinity(t *testing.T) {
	def := Definition{ChannelID: "c", Trait: &Trait{}, Affinity: &Affinity{Attr: "  ", Value: ""}}
	if root := def.Tree(); root != nil {
		t.Fatalf("empty criteria derived a root: %+v", root)
	}
}

func TestAffinityWindowFallsBackTo90(t *testing.T) {
	if got := (&Affinity{}).Window(); got != 90 {
		t.Errorf("zero days gave %d, want the 90-day default", got)
	}
	if got := (&Affinity{Days: 30}).Window(); got != 30 {
		t.Errorf("30 days gave %d", got)
	}
	if got := (*Affinity)(nil).Window(); got != 90 {
		t.Errorf("a nil affinity gave %d, want the default", got)
	}
}

// The definition is stored as JSON, so a field that does not survive a round trip is a segment that
// quietly changes meaning on the next refresh
func TestDefinitionSurvivesAJSONRoundTrip(t *testing.T) {
	in := Definition{
		ChannelID: uuid.New().String(),
		Root: &Node{Kind: KindGroup, Op: OpOr, Children: []Node{
			{Kind: KindEvent, Event: "page_view", Count: 3, Days: 14, Negate: true},
			{Kind: KindTrait, Trait: &Trait{Lifecycle: []string{"at_risk"}, MinRecencyScore: 4, MinEngagement: 50}},
			{Kind: KindAffinity, Affinity: &Affinity{Attr: "brand", Value: "acme", Days: 30, Auto: true}},
		}},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, ok := Parse(string(raw))
	if !ok {
		t.Fatal("a marshalled definition has to parse back")
	}
	got, want := out.Root.Children, in.Root.Children
	if len(got) != len(want) {
		t.Fatalf("round trip lost children: %d want %d", len(got), len(want))
	}
	if got[0].Count != 3 || got[0].Days != 14 || !got[0].Negate {
		t.Errorf("event leaf came back as %+v", got[0])
	}
	if got[1].Trait.MinRecencyScore != 4 || got[1].Trait.MinEngagement != 50 {
		t.Errorf("trait leaf came back as %+v", got[1].Trait)
	}
	if got[2].Affinity.Value != "acme" || got[2].Affinity.Days != 30 || !got[2].Affinity.Auto {
		t.Errorf("affinity leaf came back as %+v", got[2].Affinity)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Tire size: 205/55R16": "tire-size-205-55r16",
		"  Spaced  Out  ":      "spaced-out",
		"ALLCAPS":              "allcaps",
		"!!!":                  "segment",
		"":                     "segment",
		"trailing---":          "trailing",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
