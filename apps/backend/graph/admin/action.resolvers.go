package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/action"
	"github.com/skeletg/skelet/internal/auth"
)

func (r *mutationResolver) RunAdminAction(ctx context.Context, slug string, args interface{}) (*model.AdminActionResult, error) {
	def, ok := action.Find(slug)
	if !ok {
		return nil, fmt.Errorf("unknown admin action: %s", slug)
	}
	if def.RequireGlobalPermission { // a channel-scoped grant would let one tenant's admin act for every tenant
		if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, def.Permission); err != nil {
			return nil, err
		}
	} else if err := auth.RequirePermission(ctx, r.AdminSvc, def.Permission); err != nil {
		return nil, err
	}

	var rawArgs map[string]any // the JSON scalar arrives as map[string]any for objects - any other shape is a client error
	if args != nil {
		m, ok := args.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("args must be a JSON object")
		}
		rawArgs = m
	} else {
		rawArgs = map[string]any{}
	}

	actingID := uuid.Nil
	if claims, ok := auth.AdminFromContext(ctx); ok {
		actingID = claims.AdminID
	}

	start := time.Now()
	result, runErr := r.ActionSvc.Run(ctx, slug, rawArgs, actingID)
	durationMs := int32(time.Since(start).Milliseconds())

	if runErr != nil {
		if errors.Is(runErr, action.ErrUnknownAction) { // registered-then-removed race - GraphQL error, the UI already saw the definition
			return nil, runErr
		}
		return &model.AdminActionResult{ // handler/arg failures come back success=false so the UI renders them inline, no error toast
			Success:    false,
			Output:     runErr.Error(),
			DurationMs: durationMs,
		}, nil
	}
	return &model.AdminActionResult{
		Success:    true,
		Output:     result.Output,
		Data:       result.Data,
		DurationMs: durationMs,
	}, nil
}

func (r *queryResolver) AdminActions(ctx context.Context) ([]*model.AdminAction, error) {
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// superusers have no admin_user_roles rows, so AllAvailable's per-action HasPermission filter would return empty - give them everything
	var defs []action.Definition
	if claims.IsSuperuser {
		defs = action.AllDefinitions()
	} else {
		defs = action.AllAvailable(ctx, r.AdminSvc, claims.AdminID)
	}

	out := make([]*model.AdminAction, len(defs))
	for i, d := range defs {
		out[i] = toAdminActionModel(d)
	}
	return out, nil
}
