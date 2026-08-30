package blockkit

import (
	"encoding/json"
	"testing"
)

func collectIDs(blocks []Block, into *[]string) {
	for _, b := range blocks {
		*into = append(*into, b.ID)
		collectIDs(b.Blocks, into)
	}
}

func idsOf(doc Document) []string {
	var out []string
	collectIDs(doc.Blocks, &out)
	return out
}

func nestedFixture() Document { // a page-type template shape: two top-level blocks, one with nested children
	return Document{Version: 1, Blocks: []Block{
		{ID: "hero-1", Type: TypeHero, Props: map[string]any{"title": "Welcome"}},
		{ID: "section-1", Type: TypeSection, Blocks: []Block{
			{ID: "rich-1", Type: TypeRichText, Props: map[string]any{"html": "<p>hi</p>"}},
			{ID: "cols-1", Type: TypeColumns, Blocks: []Block{
				{ID: "rich-2", Type: TypeRichText},
			}},
		}},
	}}
}

func TestCloneWithNewIDsReIDsEveryBlockAtEveryDepth(t *testing.T) { // seeding a page from a template while sharing IDs would make two pages address the same blocks, so an edit to one moves the other
	src := nestedFixture()
	before := idsOf(src)

	clone := CloneWithNewIDs(src)
	after := idsOf(clone)

	if len(after) != len(before) {
		t.Fatalf("clone has %d blocks, want the source's %d", len(after), len(before))
	}
	old := map[string]bool{}
	for _, id := range before {
		old[id] = true
	}
	for _, id := range after {
		if id == "" {
			t.Error("a cloned block has no id")
		}
		if old[id] {
			t.Errorf("cloned block reuses source id %q", id)
		}
	}
	// and the new ids are distinct from each other, not one id repeated
	seen := map[string]bool{}
	for _, id := range after {
		if seen[id] {
			t.Errorf("id %q appears twice in the clone", id)
		}
		seen[id] = true
	}
}

func TestCloneWithNewIDsPreservesStructureAndProps(t *testing.T) { // only the ids change - a clone that drops props or flattens nesting silently loses the template's content
	src := nestedFixture()
	clone := CloneWithNewIDs(src)

	if clone.Version != src.Version {
		t.Errorf("version = %d, want the source's %d", clone.Version, src.Version)
	}
	if len(clone.Blocks) != 2 {
		t.Fatalf("%d top-level blocks, want 2", len(clone.Blocks))
	}
	if clone.Blocks[0].Type != TypeHero || clone.Blocks[0].Props["title"] != "Welcome" {
		t.Errorf("first block = %+v, want the hero with its props", clone.Blocks[0])
	}
	section := clone.Blocks[1]
	if section.Type != TypeSection || len(section.Blocks) != 2 {
		t.Fatalf("section = %+v, want two children", section)
	}
	if len(section.Blocks[1].Blocks) != 1 {
		t.Errorf("the grandchild depth was flattened: %+v", section.Blocks[1])
	}
	if section.Blocks[1].Blocks[0].Type != TypeRichText {
		t.Errorf("grandchild type = %q, want rich text", section.Blocks[1].Blocks[0].Type)
	}
}

func TestCloneWithNewIDsDoesNotMutateTheSource(t *testing.T) { // the template is cloned on every page create, so mutating it would corrupt the type for every later page
	src := nestedFixture()
	beforeJSON, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal source: %v", err)
	}

	_ = CloneWithNewIDs(src)

	afterJSON, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("re-marshal source: %v", err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Errorf("the source changed during cloning:\n before %s\n after  %s", beforeJSON, afterJSON)
	}
}

func TestCloneWithNewIDsOnEmptyAndNilBlocks(t *testing.T) { // an empty template is the common starting point, and nil must stay nil so the body serializes the same way
	nilDoc := CloneWithNewIDs(Document{Version: 1})
	if nilDoc.Blocks != nil {
		t.Errorf("blocks = %v, want nil preserved", nilDoc.Blocks)
	}
	if nilDoc.Version != 1 {
		t.Errorf("version = %d, want 1", nilDoc.Version)
	}

	emptyDoc := CloneWithNewIDs(Document{Version: 1, Blocks: []Block{}})
	if emptyDoc.Blocks == nil {
		t.Error("an empty slice became nil, which changes the serialized body")
	}
	if len(emptyDoc.Blocks) != 0 {
		t.Errorf("blocks = %v, want empty", emptyDoc.Blocks)
	}
}

func TestCloneWithNewIDsIsDistinctEachCall(t *testing.T) { // two pages seeded from one template must not collide with each other either
	src := nestedFixture()
	first, second := idsOf(CloneWithNewIDs(src)), idsOf(CloneWithNewIDs(src))

	seen := map[string]bool{}
	for _, id := range first {
		seen[id] = true
	}
	for _, id := range second {
		if seen[id] {
			t.Errorf("two clones of one template share id %q", id)
		}
	}
}
