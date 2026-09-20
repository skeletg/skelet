package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/core/internal/datajob"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) StartDataExport(ctx context.Context, input model.StartDataExportInput) (*model.DataJob, error) {
	if r.DataJobSvc == nil {
		return nil, errNotConfigured("import and export")
	}
	res, err := r.DataJobSvc.Registry().Get(input.Resource)
	if err != nil {
		return nil, apperr.New("DATA_UNKNOWN_RESOURCE", apperr.ClassValidation, err.Error())
	}
	channelID, err := parseOptionalChannel(input.ChannelID)
	if err != nil {
		return nil, err
	}
	// Scoped to the channel the run targets when there is one - an all-channel export needs the grant to hold everywhere
	if err := r.requireDataPermission(ctx, res.ViewPermission, channelID, res.RequireGlobalPermission); err != nil {
		return nil, err
	}
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	job, err := r.DataJobSvc.QueueExport(ctx, datajob.ExportRequest{
		Resource:  input.Resource,
		Format:    dataio.Format(input.Format),
		Options:   optionValues(input.Options),
		ChannelID: channelID,
		Locale:    derefString(input.Locale),
		AdminID:   claims.AdminID,
	})
	if err != nil {
		return nil, dataJobError(err)
	}
	return toDataJobModel(job, res.Label), nil
}

func (r *mutationResolver) CancelDataJob(ctx context.Context, id string) (*model.DataJob, error) {
	if r.DataJobSvc == nil {
		return nil, errNotConfigured("import and export")
	}
	jobID, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("job id", err)
	}
	// Cancelling is a write against whatever the job would have done, so it needs that resource's stronger grant
	job, err := r.DataJobSvc.Get(ctx, jobID)
	if err != nil {
		return nil, dataJobError(err)
	}
	if err := r.requireJobAccess(ctx, job, true); err != nil {
		return nil, err
	}
	cancelled, err := r.DataJobSvc.Cancel(ctx, jobID)
	if err != nil {
		return nil, dataJobError(err)
	}
	return toDataJobModel(cancelled, r.dataResourceLabel(cancelled.Resource)), nil
}

func (r *queryResolver) DataResources(ctx context.Context) ([]*model.DataResource, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "data.view"); err != nil {
		return nil, err
	}
	if r.DataJobSvc == nil {
		return []*model.DataResource{}, nil
	}
	all := r.DataJobSvc.Registry().List()
	out := make([]*model.DataResource, 0, len(all))
	for _, res := range all {
		if !r.holdsAnywhere(ctx, res.ViewPermission) {
			continue // a resource the caller can't read is not on their menu at all
		}
		out = append(out, toDataResourceModel(ctx, r.DataJobSvc.Registry(), res, r.holdsAnywhere(ctx, res.ImportPermission)))
	}
	return out, nil
}

func (r *queryResolver) DataResource(ctx context.Context, slug string) (*model.DataResource, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "data.view"); err != nil {
		return nil, err
	}
	if r.DataJobSvc == nil {
		return nil, nil
	}
	res, err := r.DataJobSvc.Registry().Get(slug)
	if err != nil {
		return nil, nil
	}
	if !r.holdsAnywhere(ctx, res.ViewPermission) {
		return nil, auth.ErrForbidden
	}
	return toDataResourceModel(ctx, r.DataJobSvc.Registry(), res, r.holdsAnywhere(ctx, res.ImportPermission)), nil
}

func (r *queryResolver) DataFormats(ctx context.Context) ([]*model.DataFormat, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "data.view"); err != nil {
		return nil, err
	}
	codecs := dataio.Formats()
	out := make([]*model.DataFormat, len(codecs))
	for i, c := range codecs {
		out[i] = &model.DataFormat{Format: string(c.Format()), Label: c.Label(), Extension: c.Extension()}
	}
	return out, nil
}

func (r *queryResolver) DataJobs(ctx context.Context, resources []string, direction *string, statuses []string, limit *int32, offset *int32) (*model.DataJobList, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "data.view"); err != nil {
		return nil, err
	}
	if r.DataJobSvc == nil {
		return &model.DataJobList{Items: []*model.DataJob{}}, nil
	}
	// Narrowed to what the caller may see rather than filtered after the fact, so the totalCount is honest
	visible := r.visibleResources(ctx, resources)
	if len(visible) == 0 {
		return &model.DataJobList{Items: []*model.DataJob{}}, nil
	}
	jobs, total, err := r.DataJobSvc.List(ctx, datajob.ListFilter{
		Resources: visible,
		Direction: dataio.Direction(derefString(direction)),
		Statuses:  statuses,
		Limit:     int(derefInt32Default(limit, 50)),
		Offset:    int(derefInt32Default(offset, 0)),
	})
	if err != nil {
		return nil, err
	}
	items := make([]*model.DataJob, len(jobs))
	for i, j := range jobs {
		items[i] = toDataJobModel(j, r.dataResourceLabel(j.Resource))
	}
	return &model.DataJobList{Items: items, TotalCount: int32(total)}, nil
}

func (r *queryResolver) DataJob(ctx context.Context, id string) (*model.DataJob, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "data.view"); err != nil {
		return nil, err
	}
	if r.DataJobSvc == nil {
		return nil, nil
	}
	jobID, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("job id", err)
	}
	job, err := r.DataJobSvc.Get(ctx, jobID)
	if err != nil {
		return nil, dataJobError(err)
	}
	if err := r.requireJobAccess(ctx, job, false); err != nil {
		return nil, err
	}
	return toDataJobModel(job, r.dataResourceLabel(job.Resource)), nil
}

func (r *queryResolver) DataJobErrors(ctx context.Context, id string, limit *int32, offset *int32) (*model.DataJobErrorList, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "data.view"); err != nil {
		return nil, err
	}
	if r.DataJobSvc == nil {
		return &model.DataJobErrorList{Items: []*model.DataJobError{}}, nil
	}
	jobID, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("job id", err)
	}
	job, err := r.DataJobSvc.Get(ctx, jobID)
	if err != nil {
		return nil, dataJobError(err)
	}
	if err := r.requireJobAccess(ctx, job, false); err != nil {
		return nil, err
	}
	rows, total, err := r.DataJobSvc.Errors(ctx, jobID, int(derefInt32Default(limit, 100)), int(derefInt32Default(offset, 0)))
	if err != nil {
		return nil, err
	}
	items := make([]*model.DataJobError, len(rows))
	for i, e := range rows {
		items[i] = &model.DataJobError{Line: int32(e.Line), Ref: e.Ref, Message: e.Message}
	}
	return &model.DataJobErrorList{Items: items, TotalCount: int32(total)}, nil
}
