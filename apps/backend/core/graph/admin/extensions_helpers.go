package admin

import (
	"context"
	"encoding/json"
	"maps"
	"slices"

	"github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/recordsave"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// The records whose saves carry a part for plugins
const (
	productSave  = recordsave.RecordProduct
	customerSave = recordsave.RecordCustomer
)

// A save's extensions as each plugin reads its part - an object, each plugin's part under its slug. The record's own
// permission got the admin this far; each part also has to be one a plugin declared for the record, and the admin has
// to hold whatever permission that plugin named for it
func (r *Resolver) extensionsInput(ctx context.Context, record string, v any) (map[string]json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, apperr.New("EXTENSIONS_INVALID", apperr.ClassValidation, "extensions must be an object, each plugin's part under its slug")
	}
	out := make(map[string]json.RawMessage, len(m))
	for k, part := range m {
		b, err := json.Marshal(part)
		if err != nil {
			return nil, apperr.New("EXTENSIONS_INVALID", apperr.ClassValidation, "the "+k+" extension doesn't read as JSON")
		}
		out[k] = b
	}
	perms, err := plugin.RecordExtensionPermissions(ctx, record, slices.Sorted(maps.Keys(out)))
	if err != nil {
		return nil, err
	}
	for _, perm := range perms {
		if err := auth.RequirePermission(ctx, r.AdminSvc, perm); err != nil {
			return nil, err
		}
	}
	return out, nil
}
