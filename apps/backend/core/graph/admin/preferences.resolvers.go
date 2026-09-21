package admin

import (
	"context"
	"encoding/json"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) SetMyPreference(ctx context.Context, key string, value interface{}) (bool, error) {
	claims, err := auth.RequireAuth(ctx) // self-scoped by design - integration tokens carry no admin identity and are refused here
	if err != nil {
		return false, err
	}
	if err := r.PrefsSvc.Set(ctx, claims.AdminID, key, value); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) MyPreferences(ctx context.Context) ([]*model.AdminPreference, error) {
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	prefs, err := r.PrefsSvc.List(ctx, claims.AdminID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.AdminPreference, len(prefs))
	for i, p := range prefs {
		entry := &model.AdminPreference{Key: p.Key, UpdatedAt: p.UpdatedAt}
		if len(p.Value) > 0 {
			var v any
			if json.Unmarshal(p.Value, &v) == nil {
				entry.Value = v
			}
		}
		out[i] = entry
	}
	return out, nil
}
