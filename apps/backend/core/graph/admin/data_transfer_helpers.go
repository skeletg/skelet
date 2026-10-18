package admin // the toModel converters live in a helpers file because gqlgen strips unrecognized funcs out of .resolvers.go on regeneration

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/core/internal/datajob"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *Resolver) holdsAnywhere(ctx context.Context, slug string) bool {
	return auth.HoldsAnywhere(ctx, r.AdminSvc, slug)
}

func (r *Resolver) holdsViewAnywhere(ctx context.Context, res dataio.Resource) bool {
	for _, slug := range res.ViewPermissions() {
		if !r.holdsAnywhere(ctx, slug) {
			return false
		}
	}
	return true
}

func (r *Resolver) requireViewPermission(ctx context.Context, res dataio.Resource, channelID uuid.UUID) error {
	return auth.RequireAllScopedOrGlobal(ctx, r.AdminSvc, res.ViewPermissions(), channelID, res.RequireGlobalPermission)
}

func toDataResourceModel(ctx context.Context, reg *dataio.Registry, res dataio.Resource, canImport bool) *model.DataResource {
	out := &model.DataResource{
		Slug:             res.Slug,
		Label:            res.Label,
		Group:            res.Group,
		Description:      res.Description,
		Fields:           make([]*model.DataField, len(res.Fields)),
		Options:          make([]*model.DataOption, len(res.Options)),
		CanImport:        res.CanImport() && canImport,
		CanExport:        res.CanExport(),
		ViewPermission:   res.ViewPermission,
		ViewAlsoRequires: append([]string{}, res.ViewAlsoRequires...),
		ImportPermission: res.ImportPermission,
		DynamicPrefixes:  res.DynamicPrefixes,
	}
	if out.DynamicPrefixes == nil {
		out.DynamicPrefixes = []string{}
	}
	for i, f := range res.Fields {
		enum := f.Enum
		if enum == nil {
			enum = []string{}
		}
		out.Fields[i] = &model.DataField{
			Name:        f.Name,
			Label:       f.Title(),
			Type:        string(f.Type),
			Description: f.Description,
			Example:     f.Example,
			Required:    f.Required,
			Identifier:  f.Identifier,
			ReadOnly:    f.ReadOnly,
			Enum:        enum,
		}
	}
	for i, o := range res.Options {
		// A failing provider yields an empty picker rather than failing the whole page - the run itself still
		// refuses on the missing required option, so nothing silently proceeds
		resolved, err := reg.Choices(ctx, o)
		if err != nil {
			slog.Warn("data option choices could not be loaded", "resource", res.Slug, "option", o.Name, "error", err)
		}
		choices := make([]*model.DataOptionChoice, len(resolved))
		for j, c := range resolved {
			label := c.Label
			if label == "" {
				label = c.Value
			}
			choices[j] = &model.DataOptionChoice{Value: c.Value, Label: label}
		}
		out.Options[i] = &model.DataOption{
			Name:        o.Name,
			Label:       o.Label,
			Description: o.Description,
			Type:        string(o.Type),
			Required:    o.Required,
			Default:     o.Default,
			Choices:     choices,
			AppliesTo:   string(o.Applies),
		}
	}
	return out
}

func toDataJobModel(job datajob.Job, label string) *model.DataJob {
	opts := map[string]any{}
	for k, v := range job.Options {
		opts[k] = v
	}
	raw, _ := json.Marshal(opts)
	warnings := job.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	out := &model.DataJob{
		ID:              job.ID.String(),
		Direction:       string(job.Direction),
		Resource:        job.Resource,
		ResourceLabel:   label,
		Format:          string(job.Format),
		Status:          string(job.Status),
		DryRun:          job.DryRun,
		Options:         json.RawMessage(raw),
		Filename:        job.Filename,
		HasArtifact:     job.HasArtifact(),
		ByteSize:        int32(job.ByteSize),
		TotalRows:       int32(job.TotalRows),
		ProcessedRows:   int32(job.ProcessedRows),
		CreatedRows:     int32(job.CreatedRows),
		UpdatedRows:     int32(job.UpdatedRows),
		SkippedRows:     int32(job.SkippedRows),
		FailedRows:      int32(job.FailedRows),
		ErrorSummary:    job.ErrorSummary,
		Warnings:        warnings,
		ErrorsTruncated: job.ErrorsTruncated,
		Attempts:        int32(job.Attempts),
		CreatedAt:       job.CreatedAt,
		StartedAt:       job.StartedAt,
		FinishedAt:      job.FinishedAt,
		ExpiresAt:       job.ExpiresAt,
	}
	if job.ChannelID != uuid.Nil {
		s := job.ChannelID.String()
		out.ChannelID = &s
	}
	if job.AdminID != uuid.Nil {
		s := job.AdminID.String()
		out.AdminID = &s
	}
	if job.IntegrationID != uuid.Nil {
		s := job.IntegrationID.String()
		out.IntegrationID = &s
	}
	return out
}

// A job row names its resource by slug - the label is looked up so the history reads in the operator's words
func (r *Resolver) dataResourceLabel(slug string) string {
	if r.DataJobSvc == nil {
		return slug
	}
	if res, err := r.DataJobSvc.Registry().Get(slug); err == nil {
		return res.Label
	}
	return slug
}

func optionValues(in []*model.DataOptionValue) map[string]string {
	out := make(map[string]string, len(in))
	for _, v := range in {
		if v != nil {
			out[v.Name] = v.Value
		}
	}
	return out
}

func parseOptionalChannel(raw *string) (uuid.UUID, error) {
	if raw == nil || *raw == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return uuid.Nil, errInvalidID("channelId", err)
	}
	return id, nil
}

func (r *Resolver) requireDataPermission(ctx context.Context, slug string, channelID uuid.UUID, global bool) error {
	return auth.RequireScopedOrGlobal(ctx, r.AdminSvc, slug, channelID, global)
}

// Reading a job means being allowed to read what it moved - write means the import grant, since cancelling changes an outcome
func (r *Resolver) requireJobAccess(ctx context.Context, job datajob.Job, write bool) error {
	res, err := r.DataJobSvc.Registry().Get(job.Resource)
	if err != nil {
		return auth.ErrForbidden // a job for a resource this build no longer registers stays sealed
	}
	if write && res.ImportPermission != "" {
		return r.requireDataPermission(ctx, res.ImportPermission, job.ChannelID, res.GlobalFor(job.Direction))
	}
	return r.requireViewPermission(ctx, res, job.ChannelID)
}

// requireJobAccess as a SQL filter, so the total counts only the runs the caller may read
func (r *Resolver) jobVisibility(ctx context.Context, resources []string) (*datajob.Visibility, error) {
	v := &datajob.Visibility{}
	for _, slug := range resources {
		res, err := r.DataJobSvc.Registry().Get(slug)
		if err != nil {
			continue
		}
		bypass, ids, err := r.viewChannels(ctx, res)
		if err != nil {
			return nil, err
		}
		if bypass {
			v.AnyChannel = append(v.AnyChannel, slug)
			continue
		}
		if res.RequireGlobalPermission {
			continue
		}
		for _, id := range ids {
			v.Pairs = append(v.Pairs, datajob.VisiblePair{Resource: slug, ChannelID: id})
		}
	}
	return v, nil
}

// The channels where every view grant holds: bypass only when each one bypasses, otherwise the intersection of the scoped ones
func (r *Resolver) viewChannels(ctx context.Context, res dataio.Resource) (bool, []uuid.UUID, error) {
	var scoped []auth.PermittedChannels
	for _, slug := range res.ViewPermissions() {
		perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, slug)
		if err != nil {
			return false, nil, err
		}
		if !perm.Bypass() {
			scoped = append(scoped, perm)
		}
	}
	if len(scoped) == 0 {
		return true, nil, nil
	}
	var ids []uuid.UUID
	for _, id := range scoped[0].IDs() {
		if allAllow(scoped[1:], id) {
			ids = append(ids, id)
		}
	}
	return false, ids, nil
}

func allAllow(perms []auth.PermittedChannels, id uuid.UUID) bool {
	for _, p := range perms {
		if !p.Allows(id) {
			return false
		}
	}
	return true
}

// The resources a caller may list jobs for, intersected with whatever filter they asked for
func (r *Resolver) visibleResources(ctx context.Context, requested []string) []string {
	want := map[string]bool{}
	for _, s := range requested {
		want[s] = true
	}
	var out []string
	for _, res := range r.DataJobSvc.Registry().List() {
		if len(want) > 0 && !want[res.Slug] {
			continue
		}
		if r.holdsViewAnywhere(ctx, res) {
			out = append(out, res.Slug)
		}
	}
	return out
}

func dataJobError(err error) error {
	switch {
	case errors.Is(err, datajob.ErrJobNotFound):
		return apperr.New("DATA_JOB_NOT_FOUND", apperr.ClassNotFound, "that transfer no longer exists")
	case errors.Is(err, datajob.ErrNotCancelable):
		return apperr.New("DATA_JOB_RUNNING", apperr.ClassValidation, "this run has already started")
	case errors.Is(err, datajob.ErrNoArtifact):
		return apperr.New("DATA_JOB_NO_FILE", apperr.ClassNotFound, "that file has passed its retention window")
	case errors.Is(err, datajob.ErrNoStorage):
		return apperr.New("DATA_STORAGE_MISSING", apperr.ClassValidation, err.Error())
	case errors.Is(err, dataio.ErrUnknownResource), errors.Is(err, dataio.ErrUnknownFormat),
		errors.Is(err, dataio.ErrNotImportable), errors.Is(err, dataio.ErrNotExportable):
		return apperr.New("DATA_UNSUPPORTED", apperr.ClassValidation, err.Error())
	case errors.Is(err, dataio.ErrInvalidOption):
		return apperr.New("DATA_INVALID_OPTION", apperr.ClassValidation, err.Error())
	}
	return err
}
