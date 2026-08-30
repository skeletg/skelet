package catalogio

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/dataio"
)

func TestCoerceScalar(t *testing.T) {
	tests := []struct {
		name    string
		it      catalog.InputType
		cell    string
		want    any
		wantErr bool
	}{
		{"plain text", catalog.InputTypePlainText, "Cotton", "Cotton", false},
		{"numeric", catalog.InputTypeNumeric, "0.25", 0.25, false},
		{"numeric invalid", catalog.InputTypeNumeric, "abc", nil, true},
		{"boolean true", catalog.InputTypeBoolean, "yes", true, false},
		{"boolean false", catalog.InputTypeBoolean, "0", false, false},
		{"boolean invalid", catalog.InputTypeBoolean, "maybe", nil, true},
		{"date passthrough", catalog.InputTypeDate, "2024-01-15", "2024-01-15", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coerceScalar(tt.it, tt.cell)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.cell)
				}
				return
			}
			if err != nil {
				t.Fatalf("coerceScalar: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"Cotton T-Shirt":  "cotton-t-shirt",
		"  Spaced  Out  ": "spaced-out",
		"Already-slug":    "already-slug",
		"Ünïcödé!!!":      "n-c-d",
	}
	for in, want := range tests {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitMulti(t *testing.T) {
	got := splitMulti(" red | blue ||green ")
	want := []string{"red", "blue", "green"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if len(splitMulti("")) != 0 {
		t.Error("splitMulti(\"\") should be empty")
	}
}

func TestStringifyScalar(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{"hello", "hello"},
		{true, "true"},
		{false, "false"},
		{0.25, "0.25"},
		{float64(3), "3"},
		{nil, ""},
	}
	for _, tt := range tests {
		if got := stringifyScalar(tt.in); got != tt.want {
			t.Errorf("stringifyScalar(%#v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSameOptionSet(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	current := []catalog.Attribute{{ID: a}, {ID: b}}
	if !sameOptionSet(current, []uuid.UUID{b, a}) {
		t.Error("order must not matter - a re-import would otherwise re-set options the service refuses to change")
	}
	if sameOptionSet(current, []uuid.UUID{a}) {
		t.Error("a shorter desired set is a different set")
	}
	if sameOptionSet(current, []uuid.UUID{a, uuid.New()}) {
		t.Error("same length but different members is a different set")
	}
}

func TestParseGroupedMembers(t *testing.T) {
	t.Run("skus with and without quantities", func(t *testing.T) {
		specs, err := parseGroupedMembers("TEE-1|MUG-1:2|STICKER:0.5")
		if err != nil {
			t.Fatalf("parseGroupedMembers: %v", err)
		}
		want := []groupedMemberSpec{{"TEE-1", 1}, {"MUG-1", 2}, {"STICKER", 0.5}}
		if len(specs) != len(want) {
			t.Fatalf("specs = %d, want %d", len(specs), len(want))
		}
		for i, w := range want {
			if specs[i] != w {
				t.Fatalf("specs[%d] = %+v, want %+v", i, specs[i], w)
			}
		}
	})

	t.Run("empty cell means no members", func(t *testing.T) {
		specs, err := parseGroupedMembers("")
		if err != nil || specs != nil {
			t.Fatalf("specs = %+v, err = %v; want nil, nil", specs, err)
		}
	})

	t.Run("rejects duplicate members", func(t *testing.T) {
		if _, err := parseGroupedMembers("A|A"); err == nil {
			t.Fatal("expected duplicate-member error")
		}
	})

	t.Run("rejects non-positive or garbage quantities", func(t *testing.T) {
		for _, cell := range []string{"A:0", "A:-1", "A:two"} {
			if _, err := parseGroupedMembers(cell); err == nil {
				t.Fatalf("expected quantity error for %q", cell)
			}
		}
	})

	t.Run("rejects a bare quantity with no sku", func(t *testing.T) {
		if _, err := parseGroupedMembers(":2"); err == nil {
			t.Fatal("expected missing-sku error")
		}
	})
}

// The registry validates every registration, so a malformed field set never reaches a run
func TestResourceRegisters(t *testing.T) {
	reg := dataio.NewRegistry()
	reg.Register(Resource(nil, nil, nil))
	res, err := reg.Get("products")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !res.CanImport() || !res.CanExport() {
		t.Fatal("products must be importable and exportable")
	}
}

// A variant row must not be applied before the parent it links to
func TestImporterRunsParentsFirst(t *testing.T) {
	im := &importer{known: map[string]bool{}, childSeq: map[string]int32{}}
	parent, child := dataio.NewRow(), dataio.NewRow()
	parent.Set(colSKU, "TEE")
	child.Set(colSKU, "TEE-M")
	child.Set(colParentSKU, "TEE")
	if im.Pass(parent) != passTopLevel || im.Pass(child) != passChildren {
		t.Fatal("parent_sku is what separates the passes")
	}
}
