package front

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/order"
)

// A multiselect pick snapshots one order_line_options row per value - the rebuild must fold them into ONE input per option or buildConfiguredLineOptions rejects the add as "option supplied twice" and every reorder of the line fails
func TestReorderConfiguredInputGroupsMultiselectRows(t *testing.T) {
	toppings := uuid.New()
	engraving := uuid.New()
	sprinkles := uuid.New()
	nuts := uuid.New()
	upload := uuid.New()

	line := order.OrderLine{
		ProductSku: "mug-1",
		Quantity:   2,
		Options: []order.OrderLineOption{
			{ProductOptionID: toppings, OptionValueID: &sprinkles, InputType: "multiselect"},
			{ProductOptionID: toppings, OptionValueID: &nuts, InputType: "multiselect"},
			{ProductOptionID: engraving, InputType: "field", TextValue: "for mom"},
			{ProductOptionID: upload, InputType: "file", FileURL: "https://cdn/x.png"},
		},
	}

	got := reorderConfiguredInput(line)
	if got.ProductSku != "mug-1" || got.Quantity != 2 {
		t.Fatalf("line identity mangled: %+v", got)
	}
	if len(got.Options) != 2 {
		t.Fatalf("want 2 grouped inputs (multiselect folded, file dropped), got %d: %+v", len(got.Options), got.Options)
	}
	var multi, text int
	for _, o := range got.Options {
		switch o.OptionID {
		case toppings:
			multi++
			if len(o.ValueIDs) != 2 || o.ValueIDs[0] != sprinkles || o.ValueIDs[1] != nuts {
				t.Fatalf("multiselect values not merged in snapshot order: %+v", o.ValueIDs)
			}
		case engraving:
			text++
			if o.TextValue != "for mom" {
				t.Fatalf("text pick lost: %+v", o)
			}
		case upload:
			t.Fatal("file picks must be dropped - they can't be re-added without a fresh upload")
		}
	}
	if multi != 1 || text != 1 {
		t.Fatalf("grouping wrong: multi=%d text=%d", multi, text)
	}
}
