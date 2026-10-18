package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) SuppressEmail(ctx context.Context, email string, note *string) (bool, error) {
	if err := r.requireSignals(); err != nil {
		return false, err
	}
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return false, err
	}
	if strings.TrimSpace(email) == "" {
		return false, fmt.Errorf("email is required")
	}
	detail := "added by operator"
	if note != nil && strings.TrimSpace(*note) != "" {
		detail = *note
	}
	if err := r.SignalsAPI.Suppress(ctx, email, signalsclient.SuppressionManual, detail); err != nil {
		return false, signalsErr(err)
	}
	markSignalsRelayed(ctx)
	return true, nil
}

func (r *mutationResolver) UnsuppressEmail(ctx context.Context, email string) (bool, error) {
	if err := r.requireSignals(); err != nil {
		return false, err
	}
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return false, err
	}
	if err := r.SignalsAPI.Unsuppress(ctx, email); err != nil {
		return false, signalsErr(err)
	}
	markSignalsRelayed(ctx)
	return true, nil
}

func (r *queryResolver) EmailSuppressions(ctx context.Context, reason *string, limit *int32, offset *int32) (*model.EmailSuppressionPage, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	rsn := ""
	if reason != nil {
		rsn = *reason
	}
	lim := int32(50)
	if limit != nil && *limit > 0 {
		lim = *limit
	}
	var off int32
	if offset != nil && *offset > 0 {
		off = *offset
	}
	rows, total, err := r.SignalsAPI.ListSuppressions(ctx, rsn, lim, off)
	if err != nil {
		return nil, signalsErr(err)
	}
	markSignalsRelayed(ctx)
	out := &model.EmailSuppressionPage{Items: make([]*model.EmailSuppression, 0, len(rows)), TotalCount: int32(total)}
	for _, row := range rows {
		out.Items = append(out.Items, &model.EmailSuppression{
			ID:          row.ID.String(),
			Email:       row.Email,
			Reason:      row.Reason,
			Detail:      row.Detail,
			SoftBounces: int32(row.SoftBounces),
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		})
	}
	return out, nil
}
