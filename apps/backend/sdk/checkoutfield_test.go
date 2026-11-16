package sdk

import (
	"strings"
	"testing"
)

func TestRegisterCheckoutField(t *testing.T) {
	t.Cleanup(checkoutFields.reset)
	acme, other := &Plugin{slug: "acme"}, &Plugin{slug: "other"}
	acme.RegisterCheckoutField(CheckoutField{Key: "gift_message", Label: "Gift message"})
	acme.RegisterCheckoutField(CheckoutField{Key: "wrap", Label: "Wrap it", Type: CheckoutFieldSelect, Options: []CheckoutFieldOption{{Value: "paper", Label: "Paper"}, {Value: "box", Label: "Box"}}})

	other.RegisterCheckoutField(CheckoutField{Key: "gift_message", Label: "Their gift message"})

	got := CheckoutFields()
	if len(got) != 3 || got[0].PluginSlug != "acme" || got[0].Field.Type != CheckoutFieldText || got[1].Field.Key != "wrap" {
		t.Fatalf("registered %+v", got)
	}
	if got[0].Key() != "acme:gift_message" || got[2].Key() != "other:gift_message" {
		t.Errorf("two plugins asking the same thing: %s, %s", got[0].Key(), got[2].Key())
	}
	if v, ok := acme.CheckoutAnswer(answers{"acme:gift_message": "Happy birthday"}, "gift_message"); !ok || v != "Happy birthday" {
		t.Errorf("reading its own answer: %q, %v", v, ok)
	}
	meta := placingMeta{}
	if err := acme.SetOrderMetadata(meta, "slot", "morning"); err != nil || meta["acme:slot"] != "morning" {
		t.Errorf("metadata %v, %v - want it under the plugin's own key", meta, err)
	}
	got[1].Field.Options[0].Value = "changed"
	if CheckoutFields()[1].Field.Options[0].Value != "paper" {
		t.Error("a caller's copy reached the registry")
	}

	for name, tc := range map[string]struct {
		p     *Plugin
		f     CheckoutField
		panic string
	}{
		"taken key":         {acme, CheckoutField{Key: "gift_message", Label: "x"}, "already asks"},
		"too long":          {acme, CheckoutField{Key: "essay", Label: "x", MaxLength: MaxCheckoutAnswer + 1}, "MaxLength"},
		"bad key":           {acme, CheckoutField{Key: "Gift Message", Label: "x"}, "lowercase"},
		"no label":          {acme, CheckoutField{Key: "note"}, "needs a label"},
		"unknown type":      {acme, CheckoutField{Key: "note", Label: "x", Type: "number"}, "unknown type"},
		"select, empty":     {acme, CheckoutField{Key: "note", Label: "x", Type: CheckoutFieldSelect}, "needs options"},
		"unlabelled option": {acme, CheckoutField{Key: "size", Label: "x", Type: CheckoutFieldSelect, Options: []CheckoutFieldOption{{Value: "s"}}}, "a value and a label"},
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

type answers map[string]string

func (a answers) CheckoutFieldValue(key string) (string, bool) {
	v, ok := a[key]
	return v, ok
}

type placingMeta map[string]string

func (m placingMeta) SetPlacingMetadata(key, value string) error {
	m[key] = value
	return nil
}
