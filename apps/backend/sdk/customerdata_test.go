package sdk

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterCustomerData(t *testing.T) {
	t.Cleanup(customerData.reset)
	export := func(context.Context, CustomerDataSubject) (any, error) { return nil, nil }
	acme := &Plugin{slug: "acme"}
	acme.RegisterCustomerData(CustomerData{Key: "warranties", Export: export})
	if got := CustomerDataSets(); len(got) != 1 || got[0].Key() != "acme:warranties" {
		t.Fatalf("registered %+v", got)
	}
	for name, tc := range map[string]struct {
		d     CustomerData
		panic string
	}{
		"taken key": {CustomerData{Key: "warranties", Export: export}, "already keeps"},
		"bad key":   {CustomerData{Key: "Warranties", Export: export}, "lowercase"},
		"nothing":   {CustomerData{Key: "notes"}, "an Export, an Erase"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, tc.panic) {
					t.Errorf("panic %q, want one about %q", r, tc.panic)
				}
			}()
			acme.RegisterCustomerData(tc.d)
		})
	}
}
