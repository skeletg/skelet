package plugin

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/recordsave"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func TestRecordExtensionPermissions(t *testing.T) {
	sdk.ResetForTest()
	t.Cleanup(sdk.ResetForTest)
	sdk.NewPluginForTest("acme").RegisterRecordExtension(sdk.RecordExtension{Record: sdk.RecordProduct, Permission: "acme.manage"})
	sdk.NewPluginForTest("erp").RegisterRecordExtension(sdk.RecordExtension{Record: sdk.RecordProduct})
	sdk.NewPluginForTest("off").RegisterRecordExtension(sdk.RecordExtension{Record: sdk.RecordProduct})
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(_ context.Context, slug string, _ uuid.UUID) bool { return slug != "off" }

	perms, err := RecordExtensionPermissions(context.Background(), recordsave.RecordProduct, []string{"acme", "erp"})
	if err != nil || !slices.Equal(perms, []string{"acme.manage"}) {
		t.Errorf("perms %v, %v - want acme's own permission, and nothing more for erp", perms, err)
	}
	for name, keys := range map[string][]string{
		"a plugin that declared no part":     {"unheard-of"},
		"a part of another record":           {"acme"},
		"a plugin switched off":              {"off"},
		"one good key beside one that isn't": {"erp", "typo"},
	} {
		record := recordsave.RecordProduct
		if name == "a part of another record" {
			record = recordsave.RecordCustomer
		}
		if _, err := RecordExtensionPermissions(context.Background(), record, keys); apperr.CodeOf(err) != "EXTENSION_UNKNOWN" {
			t.Errorf("%s: %v, want EXTENSION_UNKNOWN", name, err)
		}
	}
	if sdk.RecordProduct != recordsave.RecordProduct || sdk.RecordCustomer != recordsave.RecordCustomer {
		t.Error("the SDK's record names are no longer core's")
	}
}
