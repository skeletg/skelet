package sdk

import (
	"strings"
	"testing"
)

func TestRegisterCheckoutField(t *testing.T) {
	t.Cleanup(func() { checkoutFields = nil })
	acme, other := &Plugin{slug: "acme"}, &Plugin{slug: "other"}
	acme.RegisterCheckoutField(CheckoutField{Key: "gift_message", Label: "Gift message"})
	acme.RegisterCheckoutField(CheckoutField{Key: "wrap", Label: "Wrap it", Type: CheckoutFieldSelect, Options: []string{"paper", "box"}})

	got := CheckoutFields()
	if len(got) != 2 || got[0].PluginSlug != "acme" || got[0].Field.Type != CheckoutFieldText || got[1].Field.Key != "wrap" {
		t.Fatalf("registered %+v", got)
	}
	got[1].Field.Options[0] = "changed"
	if CheckoutFields()[1].Field.Options[0] != "paper" {
		t.Error("a caller's copy reached the registry")
	}

	for name, tc := range map[string]struct {
		p     *Plugin
		f     CheckoutField
		panic string
	}{
		"taken key":     {other, CheckoutField{Key: "gift_message", Label: "x"}, "already asks"},
		"bad key":       {acme, CheckoutField{Key: "Gift Message", Label: "x"}, "lowercase"},
		"no label":      {acme, CheckoutField{Key: "note"}, "needs a label"},
		"unknown type":  {acme, CheckoutField{Key: "note", Label: "x", Type: "number"}, "unknown type"},
		"select, empty": {acme, CheckoutField{Key: "note", Label: "x", Type: CheckoutFieldSelect}, "needs options"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, tc.panic) {
					t.Errorf("panic %q, want one about %q", r, tc.panic)
				}
			}()
			tc.p.RegisterCheckoutField(tc.f)
		})
	}
}
