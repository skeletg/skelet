package permission

import (
	"strings"
	"testing"
)

// The registry is global, append-only, with no reset hook, and defaults.go's init() pre-seeds the core groups first - so every test uses unique slugs and asserts additively, never "the registry is empty"
// The lone invalid-entry test runs last (Go runs tests in source order) since its bad permission permanently poisons Validate()

func TestRegisterAndFind_RoundTrips(t *testing.T) {
	RegisterGroup(Group{Slug: "rt_group", Name: "RT Group", SortOrder: 99})
	Register(Definition{Group: "rt_group", Slug: "rt.view", Name: "RT View", SortOrder: 1})

	g, ok := FindGroup("rt_group")
	if !ok {
		t.Fatal("FindGroup(rt_group) not found after RegisterGroup")
	}
	if g.Name != "RT Group" || g.SortOrder != 99 {
		t.Errorf("FindGroup round-trip wrong: %+v", g)
	}

	p, ok := Find("rt.view")
	if !ok {
		t.Fatal("Find(rt.view) not found after Register")
	}
	if p.Group != "rt_group" || p.Name != "RT View" || p.SortOrder != 1 {
		t.Errorf("Find round-trip wrong: %+v", p)
	}
}

func TestRegisterGroupIfAbsent_KeepsTheFirst(t *testing.T) {
	RegisterGroupIfAbsent(Group{Slug: "shared_group", Name: "First", SortOrder: 80})
	RegisterGroupIfAbsent(Group{Slug: "shared_group", Name: "Second", SortOrder: 81})
	if g, _ := FindGroup("shared_group"); g.Name != "First" {
		t.Errorf("a second registrant replaced the group: %+v", g)
	}
	n := 0
	for _, g := range AllGroups() {
		if g.Slug == "shared_group" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("shared_group registered %d times, want 1", n)
	}
}

func TestFind_UnknownReturnsFalse(t *testing.T) {
	if _, ok := Find("definitely.not.registered"); ok {
		t.Error("Find of an unknown slug should return ok=false")
	}
	if _, ok := FindGroup("definitely_not_a_group"); ok {
		t.Error("FindGroup of an unknown slug should return ok=false")
	}
}

func TestAllGroupsAndPermissions_AreAdditiveCopies(t *testing.T) {
	beforeGroups := len(AllGroups())
	beforePerms := len(AllPermissions())

	RegisterGroup(Group{Slug: "add_group", Name: "Add Group"})
	Register(Definition{Group: "add_group", Slug: "add.view", Name: "Add View"})

	groups := AllGroups()
	perms := AllPermissions()
	if len(groups) != beforeGroups+1 {
		t.Errorf("AllGroups grew by %d, want 1", len(groups)-beforeGroups)
	}
	if len(perms) != beforePerms+1 {
		t.Errorf("AllPermissions grew by %d, want 1", len(perms)-beforePerms)
	}

	// Mutating the returned slice must not affect the registry (it's a copy)
	if len(groups) > 0 {
		groups[0] = Group{Slug: "mutated"}
	}
	if g2 := AllGroups(); len(g2) > 0 && g2[0].Slug == "mutated" {
		t.Error("AllGroups returned a live reference, not a copy")
	}

	// The just-registered entries must be present
	if _, ok := FindGroup("add_group"); !ok {
		t.Error("add_group missing from registry")
	}
	if _, ok := Find("add.view"); !ok {
		t.Error("add.view missing from registry")
	}
}

func TestValidate_PassesForRegisteredGroup(t *testing.T) {
	RegisterGroup(Group{Slug: "valid_group", Name: "Valid Group"})
	Register(Definition{Group: "valid_group", Slug: "valid.view", Name: "Valid View"})
	// At this point every registered permission (defaults + ours) references a
	// registered group, so Validate must still pass
	if err := Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil while all permissions reference registered groups", err)
	}
}

func TestRegisterGroup_DuplicatePanics(t *testing.T) {
	RegisterGroup(Group{Slug: "dup_group", Name: "Dup Group"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate group slug")
		}
	}()
	RegisterGroup(Group{Slug: "dup_group", Name: "Dup Group Again"})
}

func TestRegister_DuplicatePanics(t *testing.T) {
	RegisterGroup(Group{Slug: "dup_perm_group", Name: "Dup Perm Group"})
	Register(Definition{Group: "dup_perm_group", Slug: "dup.perm", Name: "Dup Perm"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate permission slug")
		}
	}()
	Register(Definition{Group: "dup_perm_group", Slug: "dup.perm", Name: "Dup Perm Again"})
}

// TestValidate_RejectsUnregisteredGroup runs LAST: it registers a permission pointing at a group that was never
// registered, which permanently makes Validate() return an error for the rest of this test binary's lifetime
func TestValidate_RejectsUnregisteredGroup(t *testing.T) {
	Register(Definition{Group: "ghost_group_xyz", Slug: "ghost.view", Name: "Ghost View"})
	err := Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error for a permission referencing an unregistered group")
	}
	if !strings.Contains(err.Error(), "ghost_group_xyz") {
		t.Errorf("error %q should name the missing group %q", err.Error(), "ghost_group_xyz")
	}
	if !strings.Contains(err.Error(), "ghost.view") {
		t.Errorf("error %q should name the offending permission %q", err.Error(), "ghost.view")
	}
}
