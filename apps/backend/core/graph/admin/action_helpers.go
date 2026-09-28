package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminaction"
	"github.com/skeletg/skelet/apps/backend/core/internal/audit"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

const auditedActionOutputBytes = 2 * 1024 // keeps the row well under audit's detail cap, which would otherwise swap the whole detail for a truncation marker

func (r *mutationResolver) requireActionPermission(ctx context.Context, def adminaction.Definition) error {
	if def.RequireGlobalPermission { // a channel-scoped grant would let one tenant's admin act for every tenant
		return auth.RequireGlobalPermission(ctx, r.AdminSvc, def.Permission)
	}
	return auth.RequirePermission(ctx, r.AdminSvc, def.Permission)
}

// Callers gate first - this only shapes the args and turns a handler failure into an inline result
func (r *mutationResolver) runAdminAction(ctx context.Context, slug string, args any) (*model.AdminActionResult, error) {
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

	start := time.Now()
	result, runErr := r.ActionSvc.Run(ctx, slug, rawArgs)
	durationMs := int32(time.Since(start).Milliseconds())

	if runErr != nil {
		if errors.Is(runErr, adminaction.ErrUnknownAction) { // registered-then-removed race - GraphQL error, the UI already saw the definition
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

// One row per attempt, refused ones included - success is the handler's own outcome, which the operation interceptor can't see because a failed run is not a GraphQL error
func (r *mutationResolver) recordAdminAction(ctx context.Context, slug string, args any, res *model.AdminActionResult, err error, took time.Duration) {
	if r.AuditSvc == nil {
		return
	}
	output := ""
	switch {
	case err != nil:
		output = err.Error()
	case res != nil:
		output = res.Output
	}
	if len(output) > auditedActionOutputBytes {
		output = strings.ToValidUTF8(output[:auditedActionOutputBytes], "") + "…" // the cut can land mid-rune
	}
	detail := map[string]any{"args": args}
	if output != "" {
		detail["output"] = output
	}
	entry := audit.Entry{
		Action:     "admin_action:" + slug,
		Detail:     detail,
		Success:    err == nil && res != nil && res.Success,
		DurationMS: took.Milliseconds(),
	}
	audit.Identify(ctx, &entry)
	r.AuditSvc.Record(ctx, entry)
}
