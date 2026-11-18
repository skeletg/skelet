package sdk

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRegisterListFilter(t *testing.T) {
	t.Cleanup(listFilters.reset)
	match := func(context.Context, []string) ([]uuid.UUID, error) { return nil, nil }
	yes := []ListFilterOption{{Value: "yes", Label: "Has one"}}
	acme := &Plugin{slug: "acme"}
	acme.RegisterListFilter(ListFilter{List: ListProducts, Key: "warranty", Label: "Warranty", Options: yes, Match: match})
	acme.RegisterListFilter(ListFilter{List: ListOrders, Key: "warranty", Label: "Warranty", Options: yes, Match: match})
	if got := ListFilters(); len(got) != 2 || got[0].Key() != "acme:warranty" {
		t.Fatalf("registered %+v", got)
	}
	for name, tc := range map[string]struct {
		f     ListFilter
		panic string
	}{
		"taken":      {ListFilter{List: ListProducts, Key: "warranty", Label: "x", Options: yes, Match: match}, "already filters"},
		"no list":    {ListFilter{List: "reviews", Key: "x", Label: "x", Options: yes, Match: match}, "no list"},
		"no options": {ListFilter{List: ListCustomers, Key: "vip", Label: "VIP", Match: match}, "needs options"},
		"no match":   {ListFilter{List: ListCustomers, Key: "vip", Label: "VIP", Options: yes}, "Match func"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, tc.panic) {
					t.Errorf("panic %q, want one about %q", r, tc.panic)
				}
			}()
			acme.RegisterListFilter(tc.f)
		})
	}
}
