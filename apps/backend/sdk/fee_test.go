package sdk

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterFee(t *testing.T) {
	t.Cleanup(fees.reset)
	charge := func(context.Context, FeeRequest) (int64, error) { return 250, nil }
	acme, other := &Plugin{slug: "acme"}, &Plugin{slug: "other"}
	acme.RegisterFee(Fee{Key: "handling", Label: "Handling", Charge: charge})
	acme.RegisterFee(Fee{Key: "eco", Label: "Eco fee", TaxClass: "reduced", Charge: charge})
	other.RegisterFee(Fee{Key: "handling", Label: "Their handling", Untaxed: true, Charge: charge})

	got := Fees()
	if len(got) != 3 || got[0].Key() != "acme:handling" || got[1].Fee.TaxClass != "reduced" || got[2].Key() != "other:handling" {
		t.Fatalf("registered %+v", got)
	}
	req := FeeRequest{Answers: map[string]string{"acme:wrap": "box", "other:wrap": "paper"}}
	if v, ok := acme.FeeAnswer(req, "wrap"); !ok || v != "box" {
		t.Errorf("reading its own answer: %q, %v", v, ok)
	}

	for name, tc := range map[string]struct {
		f     Fee
		panic string
	}{
		"taken key": {Fee{Key: "handling", Label: "x", Charge: charge}, "already charges"},
		"bad key":   {Fee{Key: "Handling Fee", Label: "x", Charge: charge}, "lowercase"},
		"no label":  {Fee{Key: "wrap", Charge: charge}, "needs a label"},
		"no charge": {Fee{Key: "wrap", Label: "Wrapping"}, "Charge func"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, tc.panic) {
					t.Errorf("panic %q, want one about %q", r, tc.panic)
				}
			}()
			acme.RegisterFee(tc.f)
		})
	}
}
