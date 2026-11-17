package admin

import (
	"context"
	"sync"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

var extensionTestPlugins sync.Once

// The record's permission alone doesn't let a part through: a plugin has to have declared it, and where it named a
// permission of its own the admin has to hold that too
func TestExtensionsInputHoldsAPartToItsPluginsDeclaration(t *testing.T) {
	extensionTestPlugins.Do(func() {
		sdk.NewPluginForTest("zz-ext-open").RegisterRecordExtension(sdk.RecordExtension{Record: sdk.RecordProduct})
		sdk.NewPluginForTest("zz-ext-gated").RegisterRecordExtension(sdk.RecordExtension{Record: sdk.RecordProduct, Permission: "zz-ext.manage"})
	})
	r := &Resolver{}
	ctx := context.Background()

	got, err := r.extensionsInput(ctx, productSave, map[string]any{"zz-ext-open": map[string]any{"months": 24}})
	if err != nil || string(got["zz-ext-open"]) != `{"months":24}` {
		t.Errorf("a declared part with no permission of its own: %s, %v", got["zz-ext-open"], err)
	}
	if got, err := r.extensionsInput(ctx, productSave, nil); got != nil || err != nil {
		t.Errorf("no extensions: %v, %v", got, err)
	}
	for name, tc := range map[string]struct {
		record string
		in     any
		code   string
	}{
		"a slug nobody declared":    {productSave, map[string]any{"zz-ext-typo": 1}, "EXTENSION_UNKNOWN"},
		"a part of another record":  {customerSave, map[string]any{"zz-ext-open": 1}, "EXTENSION_UNKNOWN"},
		"not an object":             {productSave, []any{1}, "EXTENSIONS_INVALID"},
		"a part needing permission": {productSave, map[string]any{"zz-ext-gated": 1}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := r.extensionsInput(ctx, tc.record, tc.in)
			if err == nil || (tc.code != "" && apperr.CodeOf(err) != tc.code) {
				t.Errorf("got %v, want it refused as %q", err, tc.code)
			}
		})
	}
}
