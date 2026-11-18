package plugin

import (
	"context"
	"fmt"
	"slices"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// RecordExtensionPermissions is what sending these parts of a record's save takes beyond the record's own permission:
// each key has to be a plugin that declared a part of that record and is switched on where the admin is working, and
// the answer is the permissions those plugins named, for the API to check. A key that isn't one is refused rather than
// dropped - a save that quietly leaves a part out reads as one that took
func RecordExtensionPermissions(ctx context.Context, record string, keys []string) ([]string, error) {
	var perms []string
	declared := sdk.RecordExtensions()
	for _, key := range keys {
		i := slices.IndexFunc(declared, func(r sdk.RegisteredRecordExtension) bool {
			return r.PluginSlug == key && r.Extension.Record == record
		})
		if i < 0 || !isEnabledFromContext(ctx, key, graphChannelID(ctx)) {
			return nil, apperr.New("EXTENSION_UNKNOWN", apperr.ClassValidation,
				fmt.Sprintf("no plugin takes extensions.%s on a %s", key, record))
		}
		if p := declared[i].Extension.Permission; p != "" && !slices.Contains(perms, p) {
			perms = append(perms, p)
		}
	}
	return perms, nil
}
