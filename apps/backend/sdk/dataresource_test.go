package sdk

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterDataResource(t *testing.T) {
	t.Cleanup(dataResources.reset)
	export := func(context.Context, DataRun, func(DataRecord) error) error { return nil }
	imp := func(context.Context, DataRun, DataRow) (DataOutcome, error) { return DataSkipped, nil }
	ok := DataResource{Slug: "points", Label: "Points", ViewPermission: "acme.view", Fields: []DataField{{Name: "email", Identifier: true}}, Export: export}
	acme := &Plugin{slug: "acme"}
	acme.RegisterDataResource(ok)
	if got := DataResources(); len(got) != 1 || got[0].PluginSlug != "acme" {
		t.Fatalf("registered %+v", got)
	}
	bad := map[string]func(DataResource) DataResource{
		"a taken slug":       func(r DataResource) DataResource { return r },
		"an uppercase slug":  func(r DataResource) DataResource { r.Slug = "Points2"; return r },
		"no view permission": func(r DataResource) DataResource { r.Slug, r.ViewPermission = "a", ""; return r },
		"neither way":        func(r DataResource) DataResource { r.Slug, r.Export = "b", nil; return r },
		"an import with no key": func(r DataResource) DataResource {
			r.Slug, r.Import, r.ImportPermission, r.Fields = "c", imp, "acme.edit", []DataField{{Name: "email"}}
			return r
		},
		"an import with no grant": func(r DataResource) DataResource { r.Slug, r.Import = "d", imp; return r },
		"a column with a space":   func(r DataResource) DataResource { r.Slug, r.Fields = "e", []DataField{{Name: "first name"}}; return r },
		"a repeated column": func(r DataResource) DataResource {
			r.Slug, r.Fields = "f", []DataField{{Name: "email"}, {Name: "email"}}
			return r
		},
		"a column of no known type": func(r DataResource) DataResource {
			r.Slug, r.Fields = "h", []DataField{{Name: "email", Type: "currency"}}
			return r
		},
		"an empty select": func(r DataResource) DataResource {
			r.Slug, r.Options = "g", []DataOption{{Name: "tier", Type: DataOptionSelect}}
			return r
		},
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r, _ := recover().(string); !strings.HasPrefix(r, "sdk.RegisterDataResource") {
					t.Errorf("panic %q", r)
				}
			}()
			acme.RegisterDataResource(mutate(ok))
		})
	}
}
