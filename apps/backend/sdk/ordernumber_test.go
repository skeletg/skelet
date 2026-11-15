package sdk

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterOrderNumberer(t *testing.T) {
	t.Cleanup(func() { orderNumberers = nil })
	acme := &Plugin{slug: "acme"}
	acme.RegisterOrderNumberer(func(context.Context, OrderNumberRequest) (string, error) { return "A-1", nil })
	if got := OrderNumberers(); len(got) != 1 || got[0].PluginSlug != "acme" {
		t.Fatalf("registered %+v", got)
	}
	for name, register := range map[string]func(){
		"a second one": func() {
			acme.RegisterOrderNumberer(func(context.Context, OrderNumberRequest) (string, error) { return "", nil })
		},
		"nil": func() { (&Plugin{slug: "other"}).RegisterOrderNumberer(nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r, _ := recover().(string); !strings.HasPrefix(r, "sdk.RegisterOrderNumberer") {
					t.Errorf("panic %q", r)
				}
			}()
			register()
		})
	}
}
